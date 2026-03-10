package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireUUIDParam returns a middleware that validates a path parameter as a UUID.
// Uses uuid.Parse() for proper validation.
func RequireUUIDParam(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param(paramName)
		if !IsValidUUID(id) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"type":   "about:blank",
				"title":  "Invalid UUID",
				"status": http.StatusBadRequest,
				"detail": "invalid " + paramName + " format",
				"code":   "INVALID_UUID",
			})
			return
		}
		c.Next()
	}
}

// RequireUUIDQuery returns a middleware that validates a query parameter as a UUID.
// Uses uuid.Parse() for proper validation.
func RequireUUIDQuery(queryName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Query(queryName)
		if id == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"type":   "about:blank",
				"title":  "Missing Parameter",
				"status": http.StatusBadRequest,
				"detail": queryName + " parameter is required",
				"code":   "MISSING_PARAMETER",
			})
			return
		}
		if !IsValidUUID(id) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"type":   "about:blank",
				"title":  "Invalid UUID",
				"status": http.StatusBadRequest,
				"detail": "invalid " + queryName + " format",
				"code":   "INVALID_UUID",
			})
			return
		}
		c.Next()
	}
}

// IsValidUUID returns true if the string is a valid UUID.
// Uses uuid.Parse() for proper validation.
func IsValidUUID(id string) bool {
	if id == "" {
		return false
	}
	_, err := uuid.Parse(id)
	return err == nil
}
