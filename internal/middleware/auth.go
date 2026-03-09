package middleware

import (
	"net/http"
	"time"

	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/gin-gonic/gin"
)

// ContextKey is the key for storing user in context
const ContextKeyUser = "user"

// AuthMiddleware handles session-based authentication
type AuthMiddleware struct {
	sessionRepo *repository.SessionRepository
}

// NewAuthMiddleware creates a new AuthMiddleware
func NewAuthMiddleware(sessionRepo *repository.SessionRepository) *AuthMiddleware {
	return &AuthMiddleware{
		sessionRepo: sessionRepo,
	}
}

// Authenticate validates session and injects user into context
func (m *AuthMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get session ID from cookie
		sessionID, err := c.Cookie("session")
		if err != nil || sessionID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": http.StatusUnauthorized,
				"detail": "no session provided",
			})
			return
		}

		// Validate session
		session, err := m.sessionRepo.FindByID(sessionID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": http.StatusUnauthorized,
				"detail": "invalid session",
			})
			return
		}

		// Check if expired
		if session.ExpiresAt.Before(time.Now()) {
			_ = m.sessionRepo.Delete(sessionID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": http.StatusUnauthorized,
				"detail": "session expired",
			})
			return
		}

		// Get user from session
		user, err := m.sessionRepo.GetUserBySession(sessionID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": http.StatusUnauthorized,
				"detail": "user not found",
			})
			return
		}

		// Check if user is active
		if !user.IsActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": http.StatusUnauthorized,
				"detail": "account disabled",
			})
			return
		}

		// Store user in context
		c.Set(ContextKeyUser, user)
		c.Set("user_id", user.ID) // Also set user_id for handlers that use GetString
		c.Next()
	}
}

// GetUser extracts user from context
func GetUser(c *gin.Context) *model.User {
	userVal, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil
	}
	return userVal.(*model.User)
}
