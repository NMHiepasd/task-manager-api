package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"task-manager-api/internal/transport/http/middleware"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestETag_GenerateAnd304NotModified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ETag())

	router.GET("/resource", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "stable-data"})
	})

	// 1. Initial request without ETag
	req1 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	assert.Equal(t, http.StatusOK, rec1.Code)
	etag := rec1.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.Contains(t, etag, `W/"`)

	// 2. Repeat request WITH matching If-None-Match header -> must return 304 Not Modified
	req2 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.String())
	assert.Equal(t, etag, rec2.Header().Get("ETag"))

	// 3. Request with mismatched If-None-Match -> must return 200 with full body
	req3 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req3.Header.Set("If-None-Match", `W/"different-tag"`)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, rec1.Body.String(), rec3.Body.String())
}

func TestETag_IgnoresNonGetRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ETag())

	router.POST("/resource", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": 1})
	})

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Empty(t, rec.Header().Get("ETag"))
}
