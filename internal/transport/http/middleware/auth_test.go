package middleware

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/domain"
	"task-manager-api/pkg/token"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const testSecret = "12345678901234567890123456789012"

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokenMaker, err := token.NewJWTMaker(testSecret)
	require.NoError(t, err)

	userID := uuid.New()
	userEmail := "test@example.com"
	userRole := "user"

	validToken, _, err := tokenMaker.CreateAccessToken(userID, userEmail, userRole, 15*time.Minute)
	require.NoError(t, err)

	expiredToken, _, err := tokenMaker.CreateAccessToken(userID, userEmail, userRole, -time.Minute)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		setupHeader   func(req *http.Request)
		checkResponse func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK_ValidToken",
			setupHeader: func(req *http.Request) {
				req.Header.Set(AuthorizationHeaderKey, "Bearer "+validToken)
			},
			checkResponse: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
		{
			name: "Unauthorized_NoHeader",
			setupHeader: func(_ *http.Request) {
				// No header
			},
			checkResponse: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			},
		},
		{
			name: "Unauthorized_UnsupportedPrefix",
			setupHeader: func(req *http.Request) {
				req.Header.Set(AuthorizationHeaderKey, "Basic "+validToken)
			},
			checkResponse: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			},
		},
		{
			name: "Unauthorized_MalformedFormat",
			setupHeader: func(req *http.Request) {
				req.Header.Set(AuthorizationHeaderKey, "Bearer")
			},
			checkResponse: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			},
		},
		{
			name: "Unauthorized_ExpiredToken",
			setupHeader: func(req *http.Request) {
				req.Header.Set(AuthorizationHeaderKey, "Bearer "+expiredToken)
			},
			checkResponse: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestID())
			router.Use(AuthMiddleware(tokenMaker))
			router.GET("/protected", func(c *gin.Context) {
				claims, err := GetAuthPayload(c)
				require.NoError(t, err)
				require.Equal(t, userID, claims.UserID)

				uid, err := GetAuthUserID(c)
				require.NoError(t, err)
				require.Equal(t, userID, uid)

				c.Status(http.StatusOK)
			})

			recorder := httptest.NewRecorder()
			request, err := http.NewRequest(http.MethodGet, "/protected", nil)
			require.NoError(t, err)

			tc.setupHeader(request)
			router.ServeHTTP(recorder, request)
			tc.checkResponse(t, recorder)
		})
	}
}

func TestGetAuthContext_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	claims, err := GetAuthPayload(c)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.Nil(t, claims)

	uid, err := GetAuthUserID(c)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.Equal(t, uuid.Nil, uid)
}
