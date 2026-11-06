package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"task-manager-api/internal/domain"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// ErrorHandler is a centralized Gin middleware that intercepts errors registered via c.Error(err)
// and renders an RFC 9457 compliant Problem Details JSON response.
func ErrorHandler(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only handle if errors exist and response has not already been written
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		reqID := GetRequestID(c)
		path := c.Request.URL.Path

		c.Header("Content-Type", "application/problem+json")

		// 1. Validation errors from go-playground/validator
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			params := make([]domain.InvalidParam, 0, len(ve))
			for _, fe := range ve {
				params = append(params, domain.InvalidParam{
					Name:   toSnakeCase(fe.Field()),
					Reason: formatValidationReason(fe),
				})
			}
			c.JSON(http.StatusBadRequest, domain.NewValidationProblem("Input validation failed", params, path, reqID))
			return
		}

		// 2. Custom Domain ValidationErrors
		var dve domain.ValidationErrors
		var dvePtr *domain.ValidationErrors
		if errors.As(err, &dve) {
			c.JSON(http.StatusBadRequest, domain.NewValidationProblem(dve.Message, dve.Params, path, reqID))
			return
		} else if errors.As(err, &dvePtr) && dvePtr != nil {
			c.JSON(http.StatusBadRequest, domain.NewValidationProblem(dvePtr.Message, dvePtr.Params, path, reqID))
			return
		}

		// 3. Payload size limit exceeded (checked before JSON syntax error)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || errors.Is(err, domain.ErrPayloadTooLarge) || strings.Contains(err.Error(), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, domain.NewProblem(
				http.StatusRequestEntityTooLarge,
				"Request body exceeds maximum allowed size",
				"PAYLOAD_TOO_LARGE",
				path,
				reqID,
			))
			return
		}

		// 4. Malformed JSON syntax or type error
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, domain.NewProblem(
				http.StatusBadRequest,
				"Malformed JSON in request payload",
				"MALFORMED_JSON",
				path,
				reqID,
			))
			return
		}

		// 5. Domain-level business errors
		status := http.StatusInternalServerError
		code := "INTERNAL_SERVER_ERROR"
		detail := "An unexpected internal error occurred"

		switch {
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "NOT_FOUND"
			detail = err.Error()

		case errors.Is(err, domain.ErrConflict):
			status = http.StatusConflict
			code = "CONFLICT"
			detail = err.Error()

		case errors.Is(err, domain.ErrUnauthorized):
			status = http.StatusUnauthorized
			code = "UNAUTHORIZED"
			detail = err.Error()

		case errors.Is(err, domain.ErrForbidden):
			status = http.StatusForbidden
			code = "FORBIDDEN"
			detail = err.Error()

		case errors.Is(err, domain.ErrValidation):
			status = http.StatusBadRequest
			code = "VALIDATION_FAILED"
			detail = err.Error()

		case errors.Is(err, domain.ErrRateLimitExceeded):
			status = http.StatusTooManyRequests
			code = "RATE_LIMIT_EXCEEDED"
			detail = err.Error()

		default:
			logger.Error("Unhandled error in request pipeline",
				"error", err,
				"request_id", reqID,
				"path", path,
			)
		}

		c.JSON(status, domain.NewProblem(status, detail, code, path, reqID))
	}
}

func formatValidationReason(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return fmt.Sprintf("must be at least %s characters or value", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters or value", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	default:
		return fmt.Sprintf("failed validation '%s'", fe.Tag())
	}
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
