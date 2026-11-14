package middleware

import (
	"github.com/gin-gonic/gin"
)

// CorsMiddleware restricts browser cross-origin calls to the configured
// origins. If allowedOrigins contains "*", any origin is allowed (and no
// credentials are offered, since browsers ignore credentials with a "*"
// origin). Otherwise the request's Origin is echoed back only when it is
// in the list, with credentials allowed.
func CorsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowAll := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowAll {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" && allowed[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Vary", "Origin")
		}
		// The lists match what the API actually serves: GET and POST
		// routes, plus OPTIONS for the preflight itself. X-Api-Key is
		// the only auth header; there is no Authorization scheme and
		// no PUT route.
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if c.Request.Method == "OPTIONS" {
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
