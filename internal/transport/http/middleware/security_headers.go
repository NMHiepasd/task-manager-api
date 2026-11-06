package middleware

import (
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
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none';")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		if isProduction {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}
