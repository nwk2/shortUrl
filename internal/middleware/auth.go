package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ApiKeyAuth requires the X-Api-Key header to match apiKey. An empty apiKey
// disables auth entirely (all requests pass through); the caller is
// responsible for warning about that at startup.
func ApiKeyAuth(apiKey string) gin.HandlerFunc {
	if apiKey == "" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		// Constant-time compare: == short-circuits and leaks how many
		// leading bytes matched through timing.
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Api-Key")), []byte(apiKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}
