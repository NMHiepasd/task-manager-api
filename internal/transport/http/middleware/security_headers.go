package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders attaches OWASP-recommended security headers to every HTTP response.
func SecurityHeaders(isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Allow Swagger UI assets while strictly isolating all API endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/swagger") {
			h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none';")
		} else {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none';")
		}

		if isProduction {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}
