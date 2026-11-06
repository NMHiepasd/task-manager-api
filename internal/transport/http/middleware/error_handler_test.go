package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-manager-api/internal/domain"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type sampleRequest struct {
	Email string `json:"email" binding:"required,email"`
	Age   int    `json:"age" binding:"required,min=18"`
}

func setupTestErrorRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	r := gin.New()
	r.Use(RequestID())
	r.Use(ErrorHandler(logger))
	return r
}

func TestErrorHandler_ValidationErrors(t *testing.T) {
	r := setupTestErrorRouter()
	r.POST("/test-validate", func(c *gin.Context) {
		var req sampleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(err)
			return
		}
		c.Status(http.StatusOK)
	})

	// Invalid email and age < 18
	body := `{"email": "notanemail", "age": 10}`
	req, _ := http.NewRequest(http.MethodPost, "/test-validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)

	require.Equal(t, 400, problem.Status)
	require.Equal(t, "VALIDATION_FAILED", problem.Code)
	require.NotEmpty(t, problem.RequestID)
	require.Len(t, problem.InvalidParams, 2)
}

func TestErrorHandler_DomainErrors(t *testing.T) {
	r := setupTestErrorRouter()

	r.GET("/not-found", func(c *gin.Context) {
		_ = c.Error(domain.ErrProjectNotFound)
	})

	r.GET("/conflict", func(c *gin.Context) {
		_ = c.Error(domain.ErrOptimisticLockConflict)
	})

	r.GET("/unauthorized", func(c *gin.Context) {
		_ = c.Error(domain.ErrInvalidToken)
	})

	r.GET("/forbidden", func(c *gin.Context) {
		_ = c.Error(domain.ErrForbidden)
	})

	testCases := []struct {
		url          string
		expectedCode int
		expectedType string
	}{
		{"/not-found", http.StatusNotFound, "NOT_FOUND"},
		{"/conflict", http.StatusConflict, "CONFLICT"},
		{"/unauthorized", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"/forbidden", http.StatusForbidden, "FORBIDDEN"},
	}

	for _, tc := range testCases {
		req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, tc.expectedCode, rec.Code)
		var prob domain.ProblemDetails
		err := json.Unmarshal(rec.Body.Bytes(), &prob)
		require.NoError(t, err)
		require.Equal(t, tc.expectedType, prob.Code)
		require.NotEmpty(t, prob.RequestID)
	}
}

func TestErrorHandler_MalformedJSON(t *testing.T) {
	r := setupTestErrorRouter()
	r.POST("/json", func(c *gin.Context) {
		var req sampleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(err)
			return
		}
	})

	req, _ := http.NewRequest(http.MethodPost, "/json", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var prob domain.ProblemDetails
	_ = json.Unmarshal(rec.Body.Bytes(), &prob)
	require.Equal(t, "MALFORMED_JSON", prob.Code)
}

func TestErrorHandler_UnhandledError(t *testing.T) {
	r := setupTestErrorRouter()
	r.GET("/internal", func(c *gin.Context) {
		_ = c.Error(errors.New("db connection failure"))
	})

	req, _ := http.NewRequest(http.MethodGet, "/internal", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var prob domain.ProblemDetails
	_ = json.Unmarshal(rec.Body.Bytes(), &prob)
	require.Equal(t, "INTERNAL_SERVER_ERROR", prob.Code)
}
