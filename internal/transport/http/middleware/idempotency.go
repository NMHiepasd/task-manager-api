package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// InProgressLockTTL defines the maximum processing duration (short TTL: 60s)
// before an in-flight idempotency reservation is considered stale/abandoned.
// Completed responses have a long TTL (24 hours) enforced by the background cleanup worker.
const InProgressLockTTL = 1 * time.Minute

// responseBodyWriter wraps gin.ResponseWriter to intercept response body for caching.
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseBodyWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency returns a middleware implementing IETF Idempotency-Key semantics.
// It guarantees that retry mutations with the same key do not create duplicate resources.
func Idempotency(querier repository.Querier) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key == "" {
			c.Next()
			return
		}

		reqID := GetRequestID(c)
		path := c.Request.URL.Path

		// Enforce key length constraints
		if len(key) > 255 {
			c.Header("Content-Type", "application/problem+json")
			c.AbortWithStatusJSON(http.StatusBadRequest, domain.NewProblem(
				http.StatusBadRequest,
				"Idempotency-Key header must not exceed 255 characters",
				"INVALID_IDEMPOTENCY_KEY",
				path,
				reqID,
			))
			return
		}

		// Extract authenticated user ID
		userID, err := GetAuthUserID(c)
		if err != nil {
			c.Next()
			return
		}

		// Buffer request body to calculate payload checksum
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, err = io.ReadAll(c.Request.Body)
			if err != nil {
				_ = c.Error(err)
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		hash := sha256.Sum256(bodyBytes)
		paramsHash := hex.EncodeToString(hash[:])

		// Attempt atomic reservation in idempotency store
		err = querier.CreateIdempotencyKey(c.Request.Context(), repository.CreateIdempotencyKeyParams{
			Key:         key,
			UserID:      userID,
			RequestPath: path,
			ParamsHash:  paramsHash,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
				// Existing key found; check state
				existing, getErr := querier.GetIdempotencyKey(c.Request.Context(), repository.GetIdempotencyKeyParams{
					Key:    key,
					UserID: userID,
				})
				if getErr != nil {
					if errors.Is(getErr, pgx.ErrNoRows) {
						c.Next()
						return
					}
					_ = c.Error(getErr)
					c.Abort()
					return
				}

				// Check for payload mismatch tampering
				if existing.ParamsHash != paramsHash {
					c.Header("Content-Type", "application/problem+json")
					c.AbortWithStatusJSON(http.StatusConflict, domain.NewProblem(
						http.StatusConflict,
						"Idempotency key was previously used with different request parameters",
						"IDEMPOTENCY_KEY_MISMATCH",
						path,
						reqID,
					))
					return
				}

				// Check if previous request is still in-flight
				if !existing.ResponseCode.Valid {
					lockTime := existing.CreatedAt.Time
					if existing.LockedAt.Valid {
						lockTime = existing.LockedAt.Time
					}

					// Stale lock detection: if the request has been processing longer than InProgressLockTTL,
					// assume previous attempt crashed, timed out, or terminated abnormally.
					if !lockTime.IsZero() && time.Since(lockTime) > InProgressLockTTL {
						// Clean up stale in-progress key with conditional lock on locked_at so we don't accidentally
						// delete another concurrent request's newly acquired reservation.
						delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancelDel()
						lockedAt := existing.LockedAt
						if !lockedAt.Valid {
							lockedAt = existing.CreatedAt
						}
						rowsAffected, delErr := querier.DeleteStaleIdempotencyKey(delCtx, repository.DeleteStaleIdempotencyKeyParams{
							Key:      key,
							UserID:   userID,
							LockedAt: lockedAt,
						})
						if delErr != nil || rowsAffected == 0 {
							c.Header("Content-Type", "application/problem+json")
							c.AbortWithStatusJSON(http.StatusConflict, domain.NewProblem(
								http.StatusConflict,
								"A request with this idempotency key is currently processing",
								"IDEMPOTENCY_IN_PROGRESS",
								path,
								reqID,
							))
							return
						}

						// Re-reserve key for this fresh attempt
						err = querier.CreateIdempotencyKey(c.Request.Context(), repository.CreateIdempotencyKeyParams{
							Key:         key,
							UserID:      userID,
							RequestPath: path,
							ParamsHash:  paramsHash,
						})
						if err != nil {
							var pgErr *pgconn.PgError
							if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation race
								c.Header("Content-Type", "application/problem+json")
								c.AbortWithStatusJSON(http.StatusConflict, domain.NewProblem(
									http.StatusConflict,
									"A request with this idempotency key is currently processing",
									"IDEMPOTENCY_IN_PROGRESS",
									path,
									reqID,
								))
								return
							}
							_ = c.Error(err)
							c.Abort()
							return
						}
					} else {
						c.Header("Content-Type", "application/problem+json")
						c.AbortWithStatusJSON(http.StatusConflict, domain.NewProblem(
							http.StatusConflict,
							"A request with this idempotency key is currently processing",
							"IDEMPOTENCY_IN_PROGRESS",
							path,
							reqID,
						))
						return
					}
				} else {
					// Replay stored response
					c.Header("Idempotent-Replayed", "true")
					statusCode := int(existing.ResponseCode.Int32)
					contentType := "application/json"
					if statusCode >= 400 {
						contentType = "application/problem+json"
					}
					c.Data(statusCode, contentType, existing.ResponseBody)
					c.Abort()
					return
				}
			} else {
				// Unknown database error during key reservation
				_ = c.Error(err)
				c.Abort()
				return
			}
		}

		// Wrap response writer to capture output body
		writer := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer

		completed := false
		defer func() {
			if !completed {
				// If processing panicked or terminated abnormally without completing,
				// delete the reserved key so subsequent retries do not get stuck in 409 IDEMPOTENCY_IN_PROGRESS.
				delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = querier.DeleteIdempotencyKey(delCtx, repository.DeleteIdempotencyKeyParams{
					Key:    key,
					UserID: userID,
				})
			}
		}()

		c.Next()

		// If the request resulted in an error (registered in c.Errors or status >= 400 or aborted),
		// remove the reserved key so the client can retry the operation cleanly.
		if len(c.Errors) > 0 || c.Writer.Status() >= 400 || c.IsAborted() {
			delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = querier.DeleteIdempotencyKey(delCtx, repository.DeleteIdempotencyKeyParams{
				Key:    key,
				UserID: userID,
			})
			return
		}

		// Persist the terminal response for future idempotency replay (successful 2xx/3xx responses)
		statusCode := c.Writer.Status()
		if statusCode >= 200 && statusCode < 400 {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			updateErr := querier.UpdateIdempotencyKeyResponse(updateCtx, repository.UpdateIdempotencyKeyResponseParams{
				Key:          key,
				UserID:       userID,
				ResponseCode: pgtype.Int4{Int32: int32(statusCode), Valid: true},
				ResponseBody: writer.body.Bytes(),
			})
			if updateErr != nil {
				// Rollback: if updating the response fails, actively delete the key
				// so subsequent retries are not permanently locked in 409 Conflict.
				delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancelDel()
				_ = querier.DeleteIdempotencyKey(delCtx, repository.DeleteIdempotencyKeyParams{
					Key:    key,
					UserID: userID,
				})
				_ = c.Error(updateErr)
				return
			}
			completed = true
		}
	}
}
