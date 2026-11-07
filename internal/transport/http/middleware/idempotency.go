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

			// Unknown database error during key reservation
			_ = c.Error(err)
			c.Abort()
			return
		}

		// Wrap response writer to capture output body
		writer := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer

		c.Next()

		// Persist the terminal response for future idempotency replay
		statusCode := c.Writer.Status()
		if statusCode >= 200 && statusCode < 500 {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = querier.UpdateIdempotencyKeyResponse(updateCtx, repository.UpdateIdempotencyKeyResponseParams{
				Key:          key,
				UserID:       userID,
				ResponseCode: pgtype.Int4{Int32: int32(statusCode), Valid: true},
				ResponseBody: writer.body.Bytes(),
			})
		}
	}
}
