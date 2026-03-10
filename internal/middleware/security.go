package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds security response headers
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// X-Frame-Options - prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// X-Content-Type-Options - prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// X-XSS-Protection (legacy but still useful)
		c.Header("X-XSS-Protection", "1; mode=block")

		// Referrer-Policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content-Security-Policy - allow inline scripts for SvelteKit
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

		c.Next()
	}
}

// RequestSizeLimit limits request body size
func RequestSizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check Content-Length header first
		if c.Request.ContentLength > maxSize {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"type":   "about:blank",
				"title":  "Payload Too Large",
				"status": http.StatusRequestEntityTooLarge,
				"detail": "request body exceeds maximum size",
			})
			return
		}
		c.Next()
	}
}

// RateLimiter implements a simple in-memory rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// AuthRateLimiter is a separate rate limiter for auth endpoints with stricter limits
type AuthRateLimiter struct {
	mu               sync.Mutex
	loginAttempts    map[string][]time.Time // IP -> timestamps
	registerAttempts map[string][]time.Time // IP -> timestamps
	loginLimit       int
	loginWindow      time.Duration
	registerLimit    int
	registerWindow   time.Duration
}

// NewAuthRateLimiter creates a rate limiter for auth endpoints
func NewAuthRateLimiter() *AuthRateLimiter {
	return &AuthRateLimiter{
		loginAttempts:    make(map[string][]time.Time),
		registerAttempts: make(map[string][]time.Time),
		loginLimit:       5, // 5 login attempts
		loginWindow:      1 * time.Minute,
		registerLimit:    3, // 3 registrations per hour
		registerWindow:   1 * time.Hour,
	}
}

// isLoginAllowed checks if login is allowed for the given IP
func (rl *AuthRateLimiter) isLoginAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	var valid []time.Time

	// Filter to only recent attempts within window
	for _, t := range rl.loginAttempts[ip] {
		if now.Sub(t) < rl.loginWindow {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.loginLimit {
		rl.loginAttempts[ip] = valid
		return false
	}

	rl.loginAttempts[ip] = append(valid, now)
	return true
}

// isRegisterAllowed checks if registration is allowed for the given IP
func (rl *AuthRateLimiter) isRegisterAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	var valid []time.Time

	// Filter to only recent attempts within window
	for _, t := range rl.registerAttempts[ip] {
		if now.Sub(t) < rl.registerWindow {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.registerLimit {
		rl.registerAttempts[ip] = valid
		return false
	}

	rl.registerAttempts[ip] = append(valid, now)
	return true
}

// AuthRateLimiterInstance is the auth-specific rate limiter
var authRateLimiter = NewAuthRateLimiter()

// NewRateLimiter creates a rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	// Cleanup old entries periodically
	go rl.cleanup()
	return rl
}

// cleanup removes old entries periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

// isAllowed checks if the key is allowed
func (rl *RateLimiter) isAllowed(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	var valid []time.Time

	// Filter to only recent requests within window
	for _, t := range rl.requests[key] {
		if now.Sub(t) < rl.window {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

// RateLimit returns a rate limiting middleware
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(limit, window)

	return func(c *gin.Context) {
		// Use IP + user agent as key
		key := c.ClientIP() + ":" + c.Request.UserAgent()

		if !limiter.isAllowed(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"type":   "about:blank",
				"title":  "Too Many Requests",
				"status": http.StatusTooManyRequests,
				"detail": "rate limit exceeded, try again later",
			})
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Window", window.String())

		c.Next()
	}
}

// StrictCORS returns a more restrictive CORS middleware
func StrictCORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		if origin == "" {
			// No Origin header - same-origin request, allow it
			allowed = true
		} else {
			for _, o := range allowedOrigins {
				if o == origin || o == "*" {
					allowed = true
					break
				}
			}
		}

		if allowed {
			if origin != "" && origin != "*" {
				c.Header("Access-Control-Allow-Origin", origin)
			} else if origin == "*" {
				c.Header("Access-Control-Allow-Origin", "*")
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400")
			// Expose rate limit headers
			c.Header("Access-Control-Expose-Headers", "X-RateLimit-Limit, X-RateLimit-Window")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// InputSanitizer sanitizes user inputs
func InputSanitizer() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sanitize query parameters
		for key, values := range c.Request.URL.Query() {
			for i, value := range values {
				c.Request.URL.Query()[key][i] = sanitizeInput(value)
			}
		}

		// Sanitize path parameters (where possible)
		for i := range c.Params {
			c.Params[i].Value = sanitizeInput(c.Params[i].Value)
		}

		c.Next()
	}
}

// sanitizeInput performs basic input sanitization
func sanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	return input
}

// ValidateSortParams validates sort_by and sort_order parameters
func ValidateSortParams(allowedColumns []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sortBy := c.Query("sort_by")
		sortOrder := c.Query("sort_order")

		// Validate sort_by
		if sortBy != "" {
			allowed := false
			for _, col := range allowedColumns {
				if sortBy == col {
					allowed = true
					break
				}
			}
			if !allowed {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"type":   "about:blank",
					"title":  "Invalid Parameter",
					"status": http.StatusBadRequest,
					"detail": "invalid sort_by value",
				})
				return
			}
		}

		// Validate sort_order
		if sortOrder != "" {
			sortOrder = strings.ToLower(sortOrder)
			if sortOrder != "asc" && sortOrder != "desc" {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"type":   "about:blank",
					"title":  "Invalid Parameter",
					"status": http.StatusBadRequest,
					"detail": "sort_order must be 'asc' or 'desc'",
				})
				return
			}
		}

		c.Next()
	}
}

// AuthLoginRateLimit limits login attempts to prevent brute force (5 per minute)
func AuthLoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !authRateLimiter.isLoginAllowed(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"type":   "about:blank",
				"title":  "Too Many Requests",
				"status": http.StatusTooManyRequests,
				"detail": "too many login attempts, please try again later",
			})
			return
		}

		c.Next()
	}
}

// AuthRegisterRateLimit limits registration to prevent spam/abuse (3 per hour)
func AuthRegisterRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !authRateLimiter.isRegisterAllowed(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"type":   "about:blank",
				"title":  "Too Many Requests",
				"status": http.StatusTooManyRequests,
				"detail": "too many registration attempts, please try again later",
			})
			return
		}

		c.Next()
	}
}
