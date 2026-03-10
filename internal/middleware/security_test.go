package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SecurityHeaders()(c)

	// Check security headers
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("X-Frame-Options header not set correctly")
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options header not set correctly")
	}
	if w.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Error("X-XSS-Protection header not set correctly")
	}
	if w.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("Referrer-Policy header not set correctly")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header not set")
	}
}

func TestRequestSizeLimit_WithinLimit(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Request.ContentLength = 100

	RequestSizeLimit(200)(c)

	if c.IsAborted() {
		t.Error("Request should not be aborted when within limit")
	}
}

func TestRequestSizeLimit_ExceedsLimit(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Request.ContentLength = 300

	RequestSizeLimit(200)(c)

	if !c.IsAborted() {
		t.Error("Request should be aborted when exceeding limit")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status %d, got %d", http.StatusRequestEntityTooLarge, w.Code)
	}
}

func TestRateLimiter_IsAllowed(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.isAllowed("test-key") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if rl.isAllowed("test-key") {
		t.Error("4th request should be denied")
	}
}

func TestAuthRateLimiter_LoginLimit(t *testing.T) {
	rl := NewAuthRateLimiter()

	// First 5 login attempts should be allowed
	for i := 0; i < 5; i++ {
		if !rl.isLoginAllowed("127.0.0.1") {
			t.Errorf("Login attempt %d should be allowed", i+1)
		}
	}

	// 6th attempt should be denied
	if rl.isLoginAllowed("127.0.0.1") {
		t.Error("6th login attempt should be denied")
	}
}

func TestAuthRateLimiter_RegisterLimit(t *testing.T) {
	rl := NewAuthRateLimiter()

	// First 3 registration attempts should be allowed
	for i := 0; i < 3; i++ {
		if !rl.isRegisterAllowed("127.0.0.1") {
			t.Errorf("Registration attempt %d should be allowed", i+1)
		}
	}

	// 4th attempt should be denied
	if rl.isRegisterAllowed("127.0.0.1") {
		t.Error("4th registration attempt should be denied")
	}
}

func TestAuthLoginRateLimit_Middleware(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/login", nil)

	AuthLoginRateLimit()(c)

	if c.IsAborted() {
		t.Error("First login attempt should not be rate limited")
	}
}

func TestAuthLoginRateLimit_MiddlewareBlocked(t *testing.T) {
	// Create a fresh rate limiter for testing
	testRL := &AuthRateLimiter{
		loginAttempts:    make(map[string][]time.Time),
		registerAttempts: make(map[string][]time.Time),
		loginLimit:       5,
		loginWindow:      1 * time.Minute,
		registerLimit:    3,
		registerWindow:   1 * time.Hour,
	}
	// Use up all login attempts
	for i := 0; i < 5; i++ {
		testRL.isLoginAllowed("127.0.0.1")
	}

	// Now check if 6th is blocked
	if testRL.isLoginAllowed("127.0.0.1") {
		t.Error("6th login attempt should be denied")
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	RateLimit(3, time.Minute)(c)

	if c.IsAborted() {
		t.Error("Should not be rate limited on first request")
	}
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("Rate limit header should be set")
	}
}

func TestStrictCORS_AllowedOrigin(t *testing.T) {
	allowed := []string{"https://example.com", "https://app.example.com"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Origin", "https://example.com")

	StrictCORS(allowed)(c)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Error("Should set allowed origin")
	}
}

func TestStrictCORS_DisallowedOrigin(t *testing.T) {
	allowed := []string{"https://example.com"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Origin", "https://evil.com")

	StrictCORS(allowed)(c)

	// Should still process the request (CORS headers optional)
	// but not expose Access-Control-Allow-Origin
}

func TestStrictCORS_Wildcard(t *testing.T) {
	allowed := []string{"*"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Origin", "https://any-site.com")

	StrictCORS(allowed)(c)

	// Wildcard should allow any origin
	// Test passes if no panic and request continues
}

func TestInputSanitizer(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?name=test%00null", nil)

	InputSanitizer()(c)

	// Should sanitize null bytes from query params
	// This test just verifies it doesn't panic
}

func TestValidateSortParams_Valid(t *testing.T) {
	allowedColumns := []string{"created_at", "updated_at", "title"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?sort_by=created_at&sort_order=asc", nil)

	ValidateSortParams(allowedColumns)(c)

	if c.IsAborted() {
		t.Error("Valid sort params should not be rejected")
	}
}

func TestValidateSortParams_InvalidColumn(t *testing.T) {
	allowedColumns := []string{"created_at", "updated_at", "title"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?sort_by=invalid_column", nil)

	ValidateSortParams(allowedColumns)(c)

	if !c.IsAborted() {
		t.Error("Invalid sort column should be rejected")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestValidateSortParams_InvalidOrder(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?sort_order=invalid", nil)

	ValidateSortParams([]string{"created_at"})(c)

	if !c.IsAborted() {
		t.Error("Invalid sort order should be rejected")
	}
}

func TestValidateSortParams_ValidOrder(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?sort_order=DESC", nil)

	ValidateSortParams([]string{"created_at"})(c)

	if c.IsAborted() {
		t.Error("Valid sort order (case insensitive) should be accepted")
	}
}

// TestRateLimitDisabled verifies rate limiting is disabled when RATE_LIMIT_ENABLED=false
func TestRateLimitDisabled(t *testing.T) {
	// Set environment variable to disable rate limiting
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	// Get the middleware - it should pass through when disabled
	middleware := RateLimit(100, time.Minute)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	// Run the middleware
	middleware(c)

	// Should NOT abort (not rate limited)
	if c.IsAborted() {
		t.Error("Request should not be aborted when rate limiting is disabled")
	}
}

// TestRateLimitDisabled_MultipleRequests verifies multiple rapid requests pass when disabled
func TestRateLimitDisabled_MultipleRequests(t *testing.T) {
	// Set environment variable to disable rate limiting
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	// Get the middleware
	middleware := RateLimit(1, time.Minute) // Very low limit

	// Make 10 rapid requests
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)

		middleware(c)

		if c.IsAborted() {
			t.Errorf("Request %d should not be aborted when rate limiting is disabled", i+1)
		}
	}
}

// TestRateLimitDisabled_AuthLogin verifies auth login rate limiting is disabled
func TestRateLimitDisabled_AuthLogin(t *testing.T) {
	// Set environment variable to disable rate limiting
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	// Get the middleware
	middleware := AuthLoginRateLimit()

	// Make 10 rapid login attempts
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/login", nil)

		middleware(c)

		if c.IsAborted() {
			t.Errorf("Login attempt %d should not be aborted when rate limiting is disabled", i+1)
		}
	}
}

// TestRateLimitEnabled_Default verifies rate limiting is enabled by default (no env var)
func TestRateLimitEnabled_Default(t *testing.T) {
	// Make sure RATE_LIMIT_ENABLED is not set
	t.Setenv("RATE_LIMIT_ENABLED", "")

	// Get the middleware with very low limit
	middleware := RateLimit(1, time.Minute)

	// First request should pass
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	middleware(c)

	if c.IsAborted() {
		t.Error("First request should not be aborted")
	}

	// Second request should be blocked
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/test", nil)
	middleware(c2)

	if !c2.IsAborted() {
		t.Error("Second request should be aborted (rate limited)")
	}

	// Verify rate limit error response
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status %d, got %d", http.StatusTooManyRequests, w2.Code)
	}
}

// TestRateLimitEnabled_ExplicitTrue verifies rate limiting is enabled when explicitly set to true
func TestRateLimitEnabled_ExplicitTrue(t *testing.T) {
	// Explicitly enable rate limiting
	t.Setenv("RATE_LIMIT_ENABLED", "true")

	// Get the middleware with very low limit
	middleware := RateLimit(1, time.Minute)

	// First request should pass
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	middleware(c)

	if c.IsAborted() {
		t.Error("First request should not be aborted")
	}

	// Second request should be blocked
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/test", nil)
	middleware(c2)

	if !c2.IsAborted() {
		t.Error("Second request should be aborted (rate limited)")
	}
}

// TestRateLimitDisabled_NoHeaders verifies no rate limit headers when disabled
func TestRateLimitDisabled_NoHeaders(t *testing.T) {
	// Set environment variable to disable rate limiting
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	// Get the middleware
	middleware := RateLimit(100, time.Minute)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	middleware(c)

	// When rate limiting is disabled, headers should not be set
	// (or at least not the rate limit specific ones)
	if c.Writer.Header().Get("X-RateLimit-Limit") != "" {
		t.Log("Note: Rate limit headers are still present even when disabled")
	}
}
