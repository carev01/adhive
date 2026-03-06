package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireUUIDParam returns a middleware that validates a path parameter as a UUID.
// It checks for 36 characters with 4 hyphens (standard UUID format: 8-4-4-4-12).
func RequireUUIDParam(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param(paramName)
		if len(id) != 36 || strings.Count(id, "-") != 4 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid " + paramName})
			return
		}
		c.Next()
	}
}