package middleware

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// CSRFCookieName is the name of the CSRF cookie
	CSRFCookieName = "csrf_token"
	// CSRFHeaderName is the header name for CSRF token
	CSRFHeaderName = "X-CSRF-Token"
)

// CSRFConfig holds CSRF configuration
type CSRFConfig struct {
	CookieName   string
	CookiePath   string
	CookieDomain string
	Secure       bool
	SameSite     string
	MaxAge       int
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() CSRFConfig {
	// CSRF is disabled by default - enable via CSRF_ENABLED=true for production
	// This allows development without CSRF token handling
	csrfEnabled := os.Getenv("CSRF_ENABLED")
	if csrfEnabled != "true" {
		// Return a config that will skip validation
		return CSRFConfig{
			CookieName: CSRFCookieName,
			CookiePath: "/",
			Secure:     false,
			SameSite:   "Disabled",
			MaxAge:     0,
		}
	}

	// Check for explicit Secure mode setting
	// Set CSRF_SECURE=false for development (HTTP), CSRF_SECURE=true for production (HTTPS)
	secureMode := os.Getenv("CSRF_SECURE")
	secure := false // Default to false for HTTP/localhost
	if secureMode == "true" {
		secure = true
	}

	// SameSite: "Strict" blocks cross-origin, "Lax" allows some cross-origin
	// For development with frontend on different port, use "Lax"
	sameSite := "Lax" // Default to Lax for cross-port development
	if os.Getenv("CSRF_SAME_SITE") == "strict" {
		sameSite = "Strict"
	}

	return CSRFConfig{
		CookieName: CSRFCookieName,
		CookiePath: "/",
		Secure:     secure,
		SameSite:   sameSite,
		MaxAge:     3600 * 24, // 24 hours
	}
}

// CSRF returns a CSRF protection middleware
func CSRF() gin.HandlerFunc {
	config := DefaultCSRFConfig()

	return func(c *gin.Context) {
		// Skip if CSRF is disabled
		if config.SameSite == "Disabled" {
			c.Next()
			return
		}

		// Debug: Log CSRF validation attempt
		if os.Getenv("DEBUG") == "true" {
			_, hasCookie := c.Cookie(config.CookieName)
			log.Printf("CSRF Check: Method=%s, Path=%s, HasTokenHeader=%v, HasTokenCookie=%v",
				c.Request.Method, c.Request.URL.Path,
				c.GetHeader(CSRFHeaderName) != "",
				hasCookie)
		}

		// Skip safe methods
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" ||
			c.Request.Method == "OPTIONS" || c.Request.Method == "TRACE" {
			c.Next()
			return
		}

		// Check for CSRF token in header
		tokenHeader := c.GetHeader(CSRFHeaderName)
		if tokenHeader == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Missing",
				"status": http.StatusForbidden,
				"detail": "CSRF token is required for this request",
			})
			return
		}

		// Get token from cookie
		tokenCookie, err := c.Cookie(config.CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Missing",
				"status": http.StatusForbidden,
				"detail": "CSRF token cookie not found",
			})
			return
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(tokenCookie)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Invalid",
				"status": http.StatusForbidden,
				"detail": "CSRF token is invalid",
			})
			return
		}

		c.Next()
	}
}

// CSRFTokenHandler returns a handler that generates CSRF tokens
func CSRFTokenHandler() gin.HandlerFunc {
	config := DefaultCSRFConfig()

	return func(c *gin.Context) {
		// Generate new token
		token := uuid.New().String()

		// Set cookie (HttpOnly to prevent JavaScript access)
		c.SetCookie(
			config.CookieName,
			token,
			config.MaxAge,
			config.CookiePath,
			config.CookieDomain,
			config.Secure,
			true, // HttpOnly
		)

		// Return token in response for non-JS clients
		c.JSON(http.StatusOK, gin.H{
			"token": token,
		})
	}
}

// CSRFWithConfig returns a CSRF middleware with custom configuration
func CSRFWithConfig(config CSRFConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip safe methods
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" ||
			c.Request.Method == "OPTIONS" || c.Request.Method == "TRACE" {
			c.Next()
			return
		}

		// Check for CSRF token in header
		tokenHeader := c.GetHeader(CSRFHeaderName)
		if tokenHeader == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Missing",
				"status": http.StatusForbidden,
				"detail": "CSRF token is required for this request",
			})
			return
		}

		// Get token from cookie
		tokenCookie, err := c.Cookie(config.CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Missing",
				"status": http.StatusForbidden,
				"detail": "CSRF token cookie not found",
			})
			return
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(tokenCookie)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"type":   "about:blank",
				"title":  "CSRF Token Invalid",
				"status": http.StatusForbidden,
				"detail": "CSRF token is invalid",
			})
			return
		}

		c.Next()
	}
}

// initCSRFToken initializes CSRF token if not present
func initCSRFToken(c *gin.Context) {
	config := DefaultCSRFConfig()

	// Check if token already exists
	_, err := c.Cookie(config.CookieName)
	if err == nil {
		// Token exists, don't regenerate
		return
	}

	// Generate new token
	token := uuid.New().String()

	// Set cookie
	c.SetCookie(
		config.CookieName,
		token,
		config.MaxAge,
		config.CookiePath,
		config.CookieDomain,
		config.Secure,
		true, // HttpOnly
	)
}

// CSRFInit initializes CSRF token on requests that need it
func CSRFInit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize token for all non-safe requests that don't have one
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" &&
			c.Request.Method != "OPTIONS" && c.Request.Method != "TRACE" {
			initCSRFToken(c)
		}
		c.Next()
	}
}
