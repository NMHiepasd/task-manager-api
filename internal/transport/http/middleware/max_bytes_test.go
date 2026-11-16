package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMaxBytesMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	r := gin.New()
	r.Use(RequestID())
	r.Use(ErrorHandler(logger))
	r.Use(MaxBytes(20)) // 20 bytes limit

	r.POST("/upload", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			_ = c.Error(err)
			return
		}
		c.String(http.StatusOK, string(body))
	})

	t.Run("PayloadUnderLimit", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/upload", strings.NewReader("short body"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "short body", rec.Body.String())
	})

	t.Run("PayloadExceedsLimit", func(t *testing.T) {
		largePayload := strings.Repeat("A", 100)
		req, _ := http.NewRequest(http.MethodPost, "/upload", strings.NewReader(largePayload))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}

func TestRequestSizeLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	r := gin.New()
	r.Use(RequestID())
	r.Use(ErrorHandler(logger))
	r.Use(RequestSizeLimiter(20, 100)) // 20 bytes standard, 100 bytes for attachments

	echoHandler := func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			_ = c.Error(err)
			return
		}
		c.String(http.StatusOK, string(body))
	}

	r.POST("/api/v1/tasks", echoHandler)
	r.POST("/api/v1/tasks/123/attachments", echoHandler)

	t.Run("StandardRoute_UnderLimit", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader("short payload"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("StandardRoute_ExceedsLimit", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(strings.Repeat("B", 30)))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("AttachmentRoute_AllowedUpToUploadLimit", func(t *testing.T) {
		// 50 bytes exceeds standard limit (20), but is under upload limit (100)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/tasks/123/attachments", strings.NewReader(strings.Repeat("C", 50)))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("AttachmentRoute_ExceedsUploadLimit", func(t *testing.T) {
		// 150 bytes exceeds upload limit (100)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/tasks/123/attachments", strings.NewReader(strings.Repeat("D", 150)))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}
