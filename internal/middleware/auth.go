package middleware

import (
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
		if c.GetHeader("X-Api-Key") != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}
