package middleware

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type etagResponseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *etagResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return len(b), nil
}

func (w *etagResponseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return len(s), nil
}

func (w *etagResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

// ETag implements conditional HTTP request handling using weak ETags (RFC 7232).
// For GET requests that yield 200 OK, it calculates an ETag checksum from the response body.
// If the client's If-None-Match header matches the computed ETag, it terminates with HTTP 304 Not Modified.
func ETag() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}

		writer := &etagResponseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}
		c.Writer = writer

		c.Next()

		statusCode := writer.statusCode
		bodyBytes := writer.body.Bytes()

		if statusCode == http.StatusOK && len(bodyBytes) > 0 {
			hash := sha256.Sum256(bodyBytes)
			etag := fmt.Sprintf(`W/"%x-%x"`, len(bodyBytes), hash[:8])
			writer.ResponseWriter.Header().Set("ETag", etag)

			if clientETag := c.GetHeader("If-None-Match"); clientETag == etag || clientETag == "*" {
				writer.ResponseWriter.WriteHeader(http.StatusNotModified)
				return
			}
		}

		writer.ResponseWriter.WriteHeader(statusCode)
		_, _ = writer.ResponseWriter.Write(bodyBytes)
	}
}
