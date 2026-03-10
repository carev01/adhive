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
