package http_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-manager-api/internal/config"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"task-manager-api/pkg/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transportHttp "task-manager-api/internal/transport/http"
)

type mockAuthService struct {
	service.AuthService
}

type mockUserService struct {
	service.UserService
}

func setupTestRouter() http.Handler {
	cfg := &config.Config{
		AppEnv:             "test",
		GinMode:            "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		RateLimitAuthRate:  1,
		RateLimitAuthBurst: 2,
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tokenMaker, _ := token.NewJWTMaker("12345678901234567890123456789012")

	return transportHttp.NewRouter(transportHttp.RouterDependencies{
		Config:      cfg,
		Logger:      log,
		DBPool:      nil, // uninitialized DB pool for testing probes
		AuthService: &mockAuthService{},
		UserService: &mockUserService{},
		TokenMaker:  tokenMaker,
	})
}

func TestRouter_Liveness(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(middleware.RequestIDHeader))

	var resp struct {
		Status string `json:"status"`
	}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "alive", resp.Status)
}

func TestRouter_Readiness_UninitializedPool(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, problem.Status)
	assert.Equal(t, "DATABASE_UNAVAILABLE", problem.Code)
	assert.NotEmpty(t, problem.RequestID)
}

func TestRouter_NoRoute_404_ProblemDetails(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/non-existent-endpoint", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, problem.Status)
	assert.Equal(t, "ROUTE_NOT_FOUND", problem.Code)
	assert.NotEmpty(t, problem.RequestID)
}

func TestRouter_NoMethod_405_ProblemDetails(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, problem.Status)
	assert.Equal(t, "METHOD_NOT_ALLOWED", problem.Code)
	assert.NotEmpty(t, problem.RequestID)
}

func TestRouter_MaxBytes_413(t *testing.T) {
	router := setupTestRouter()

	// 2MB + 1KB JSON payload
	largePayload := `{"email":"` + strings.Repeat("a", 2<<20+1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(largePayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, problem.Status)
	assert.Equal(t, "PAYLOAD_TOO_LARGE", problem.Code)
}

func TestRouter_ProtectedEndpoint_Unauthorized(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, problem.Status)
	assert.Equal(t, "UNAUTHORIZED", problem.Code)
}

func TestRouter_SecurityHeaders_And_CORS(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Security headers
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	// CORS header
	assert.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestRouter_RateLimit_Auth(t *testing.T) {
	router := setupTestRouter()

	// 2 requests within burst should pass (failing on validation 400 is fine, they are not 429)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.50:50000"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code) // validation failure, meaning rate limit allowed it through
	}

	// 3rd request should exceed rate limit and return 429
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.50:50000"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	var problem domain.ProblemDetails
	err := json.Unmarshal(rec.Body.Bytes(), &problem)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, problem.Status)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", problem.Code)
}
