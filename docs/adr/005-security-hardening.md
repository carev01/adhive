# ADR-005: Security Architecture Review & Hardening Plan

**Status:** Proposed  
**Date:** 2026-03-09  
**Decision Makers:** @bumblebee, @jarvis  
**Related ADRs:** ADR-004 (Error Handling)

---

## Executive Summary

This ADR provides a comprehensive security review of AdHive's current architecture and proposes a hardening plan. The review covers authentication, authorization, input validation, file handling, API security, and security headers.

**Overall Security Posture:** ⚠️ **Moderate** - Basic protections in place, several critical gaps identified.

---

## Current State Analysis

### Security Inventory

| Area | Current Implementation | Status |
|------|------------------------|--------|
| **Authentication** | Session-based cookies, bcrypt passwords | ⚠️ Needs hardening |
| **Authorization** | User ownership validation | ✅ Adequate |
| **Path Traversal** | Multi-layer protection | ✅ Strong |
| **Input Validation** | Basic validation in handlers | ⚠️ Inconsistent |
| **Password Hashing** | bcrypt cost 12 | ✅ Strong |
| **CORS** | Allows all origins (`*`) | ❌ Critical |
| **CSRF** | Not implemented | ❌ Critical |
| **Rate Limiting** | Not implemented | ❌ High |
| **Security Headers** | Partial (archives only) | ⚠️ Incomplete |
| **Cookie Security** | Default settings | ❌ High |
| **SQL Injection** | GORM parameterized queries | ✅ Protected |
| **XSS Prevention** | None explicit | ⚠️ Medium |
| **File Upload** | Path validation, no content validation | ⚠️ Medium |

---

## Detailed Findings

### 1. Authentication & Session Management

**Current Implementation:**
```go
// internal/middleware/auth.go
sessionID, err := c.Cookie("session")
if err != nil || sessionID == "" {
    c.AbortWithStatusJSON(http.StatusUnauthorized, ...)
    return
}
```

**Issues Identified:**

| Issue | Severity | Description |
|-------|----------|-------------|
| Cookie security flags missing | High | No HttpOnly, Secure, SameSite flags set |
| No session regeneration | Medium | Session ID doesn't rotate after login |
| No session invalidation | Medium | Old sessions not invalidated on password change |
| Session TTL fixed | Low | 7-day TTL, no sliding expiration |

**Current Cookie Setting (handler/auth.go):**
```go
c.SetCookie("session", sessionID, int(h.sessionTTL.Seconds()), "/", "", false, false)
//                                                             ↑   ↑    ↑
//                                                        Secure HttpOnly
//                                                        Missing! Missing!
```

**Recommendation:**
```go
c.SetCookie("session", sessionID, 
    int(h.sessionTTL.Seconds()), 
    "/",           // Path
    "",",          // Domain (empty = current domain)
    true,          // Secure - HTTPS only
    true,          // HttpOnly - Not accessible via JavaScript
)
// Add SameSite attribute separately via header
c.Header("Set-Cookie", c.GetString("Set-Cookie")+"; SameSite=Strict")
```

---

### 2. Authorization

**Current Implementation:**
```go
// All handlers check user ownership
user := middleware.GetUser(c)
entry, err := h.entryRepo.GetByID(ctx, id)
if entry.UserID != user.ID {
    c.AbortWithStatusJSON(http.StatusForbidden, ...)
}
```

**Assessment:** ✅ **Adequate**

User ownership is validated on all resource operations. No role-based access control (RBAC) is needed for single-user deployments.

**Minor Recommendations:**
- Add a centralized authorization helper to reduce boilerplate
- Consider adding audit logging for sensitive operations

---

### 3. Input Validation

**Current Implementation:**

| Input Type | Validation | Status |
|------------|------------|--------|
| UUID parameters | Basic format check (36 chars, 4 hyphens) | ⚠️ Weak |
| Email | Regex validation | ✅ Adequate |
| Password | Complexity rules (3 of 4) | ✅ Strong |
| URL | Gin binding validation | ⚠️ Inconsistent |
| File paths | Multi-layer traversal protection | ✅ Strong |

**UUID Validation Issue:**
```go
// internal/middleware/uuid.go
func RequireUUIDParam(paramName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Param(paramName)
        if len(id) != 36 || strings.Count(id, "-") != 4 {
            c.AbortWithStatusJSON(http.StatusBadRequest, ...)
            return
        }
        c.Next()
    }
}
```

**Problem:** Only checks format, doesn't validate it's a valid UUID.

**Recommendation:**
```go
func RequireUUIDParam(paramName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Param(paramName)
        if _, err := uuid.Parse(id); err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "error": "invalid " + paramName,
                "code":  "INVALID_UUID",
            })
            return
        }
        c.Next()
    }
}
```

---

### 4. Path Traversal Protection

**Current Implementation:** ✅ **Strong**

Multiple layers of protection:

```go
// internal/middleware/common.go
func RawPathTraversalGuard() gin.HandlerFunc {
    // Checks for:
    // - ../ patterns
    // - URL-encoded variants (%2e%2e, %2f, %5c)
    // - Double-encoded variants
    // - Both raw and decoded paths
}

// internal/handler/file.go
func cleanArchivePathPart(v string) (string, bool) {
    // Additional path sanitization:
    // - URL decoding (up to 3 iterations)
    // - Traversal token detection
    // - filepath.Clean()
    // - Absolute path rejection
}
```

**Assessment:** The path traversal protection is comprehensive and well-implemented.

---

### 5. CORS Configuration

**Current Implementation:** ❌ **Critical Issue**

```go
// internal/middleware/common.go
func CORS() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")  // DANGER: Allows any origin
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
        c.Header("Access-Control-Max-Age", "86400")
        // ...
    }
}
```

**Problems:**
1. `Access-Control-Allow-Origin: *` allows any website to make requests
2. Credentials are sent with cross-origin requests
3. No explicit allowlist of trusted origins

**Recommendation:**
```go
func CORS(allowedOrigins []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        origin := c.GetHeader("Origin")
        
        // Check if origin is in allowlist
        allowed := false
        for _, o := range allowedOrigins {
            if o == origin || (o == "*" && len(allowedOrigins) == 1) {
                allowed = true
                break
            }
        }
        
        if allowed {
            c.Header("Access-Control-Allow-Origin", origin)
            c.Header("Access-Control-Allow-Credentials", "true")
            c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept")
            c.Header("Access-Control-Max-Age", "86400")
        }
        
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)
            return
        }
        
        c.Next()
    }
}
```

---

### 6. CSRF Protection

**Current Implementation:** ❌ **Missing**

The application uses session-based authentication with cookies but has no CSRF protection.

**Risk:** An attacker can craft a malicious page that submits requests to AdHive on behalf of an authenticated user.

**Recommendation:** Implement CSRF tokens:

```go
// internal/middleware/csrf.go

package middleware

import (
    "crypto/subtle"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

const (
    CSRFTokenHeader = "X-CSRF-Token"
    CSRFTokenCookie = "csrf_token"
)

func CSRF() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Skip safe methods
        if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
            c.Next()
            return
        }
        
        // Get token from cookie and header
        cookieToken, err := c.Cookie(CSRFTokenCookie)
        if err != nil {
            c.AbortWithStatusJSON(403, gin.H{"error": "CSRF token missing"})
            return
        }
        
        headerToken := c.GetHeader(CSRFTokenHeader)
        if headerToken == "" {
            c.AbortWithStatusJSON(403, gin.H{"error": "CSRF token missing"})
            return
        }
        
        // Constant-time comparison
        if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
            c.AbortWithStatusJSON(403, gin.H{"error": "CSRF token invalid"})
            return
        }
        
        c.Next()
    }
}

func GenerateCSRFToken() string {
    return uuid.New().String()
}
```

**Integration:**
```go
// In auth handler after successful login:
csrfToken := middleware.GenerateCSRFToken()
c.SetCookie("csrf_token", csrfToken, 86400, "/", "", true, true)

// In router setup:
r.Use(middleware.CSRF())
```

---

### 7. Security Headers

**Current Implementation:** ⚠️ **Partial**

Only archive file responses have security headers:

```go
// internal/handler/file.go
func setArchiveSecurityHeaders(c *gin.Context) {
    c.Header("X-Content-Type-Options", "nosniff")
    c.Header("Cross-Origin-Resource-Policy", "same-origin")
    c.Header("Cross-Origin-Opener-Policy", "same-origin")
    c.Header("Referrer-Policy", "no-referrer")
    c.Header("Content-Security-Policy", "...")
}
```

**Missing Headers for All Responses:**

| Header | Purpose | Recommendation |
|--------|---------|----------------|
| `X-Content-Type-Options` | Prevent MIME sniffing | `nosniff` |
| `X-Frame-Options` | Prevent clickjacking | `DENY` or `SAMEORIGIN` |
| `X-XSS-Protection` | XSS filter (legacy) | `1; mode=block` |
| `Strict-Transport-Security` | Force HTTPS | `max-age=31536000; includeSubDomains` |
| `Content-Security-Policy` | XSS prevention | Configure per endpoint |
| `Referrer-Policy` | Control referrer info | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | Restrict browser features | Disable unnecessary features |

**Recommendation:**
```go
// internal/middleware/security_headers.go

package middleware

import "github.com/gin-gonic/gin"

func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Prevent MIME type sniffing
        c.Header("X-Content-Type-Options", "nosniff")
        
        // Prevent clickjacking
        c.Header("X-Frame-Options", "DENY")
        
        // XSS protection (legacy browsers)
        c.Header("X-XSS-Protection", "1; mode=block")
        
        // Referrer policy
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        
        // Permissions policy
        c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
        
        // HSTS (only if HTTPS is enabled)
        // c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        
        // CSP - varies by endpoint type
        c.Header("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
        
        c.Next()
    }
}
```

---

### 8. Rate Limiting

**Current Implementation:** ❌ **Missing**

No rate limiting is implemented, making the application vulnerable to:
- Brute force attacks on login
- DoS attacks on expensive endpoints
- Resource exhaustion

**Recommendation:**
```go
// internal/middleware/rate_limit.go

package middleware

import (
    "net/http"
    "sync"
    "time"
    
    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

type RateLimiter struct {
    visitors map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
    return &RateLimiter{
        visitors: make(map[string]*rate.Limiter),
        rate:     r,
        burst:    burst,
    }
}

func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    limiter, exists := rl.visitors[ip]
    if !exists {
        limiter = rate.NewLimiter(rl.rate, rl.burst)
        rl.visitors[ip] = limiter
    }
    
    return limiter
}

func (rl *RateLimiter) Cleanup() {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    rl.visitors = make(map[string]*rate.Limiter)
}

func RateLimit(limiter *RateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        ip := c.ClientIP()
        
        if !limiter.GetLimiter(ip).Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "rate limit exceeded",
                "code":  "RATE_LIMITED",
            })
            return
        }
        
        c.Next()
    }
}

// AuthRateLimit creates a stricter rate limiter for auth endpoints
func AuthRateLimit() gin.HandlerFunc {
    // 5 requests per minute per IP
    limiter := NewRateLimiter(rate.Every(time.Minute/5), 5)
    
    // Cleanup every 10 minutes
    go func() {
        for range time.Tick(10 * time.Minute) {
            limiter.Cleanup()
        }
    }()
    
    return RateLimit(limiter)
}
```

**Integration:**
```go
// In router setup:
auth := api.Group("/auth")
auth.POST("/login", middleware.AuthRateLimit(), authHandler.Login)
auth.POST("/register", middleware.AuthRateLimit(), authHandler.Register)
```

---

### 9. File Upload Security

**Current Implementation:** ⚠️ **Partial**

**Existing Protections:**
- Path traversal protection (strong)
- UUID validation for entry IDs
- File path sanitization

**Missing Protections:**
- Content type validation
- File size limits
- Magic byte verification
- Virus scanning (optional)

**Recommendation:**
```go
// internal/service/file_validation.go

package service

import (
    "errors"
    "mime/multipart"
    "net/http"
)

var (
    ErrFileTooLarge   = errors.New("file too large")
    ErrInvalidType    = errors.New("invalid file type")
)

const (
    MaxArchiveSize   = 100 * 1024 * 1024 // 100 MB
    MaxThumbnailSize = 10 * 1024 * 1024  // 10 MB
)

var AllowedArchiveTypes = map[string]bool{
    "application/zip":    true,
    "application/x-zip":  true,
    "application/gzip":   true,
    "application/x-gzip": true,
}

var AllowedThumbnailTypes = map[string]bool{
    "image/jpeg": true,
    "image/png":  true,
    "image/webp": true,
    "image/gif":  true,
}

func ValidateArchive(file *multipart.FileHeader) error {
    if file.Size > MaxArchiveSize {
        return ErrFileTooLarge
    }
    
    // Check content type
    f, err := file.Open()
    if err != nil {
        return err
    }
    defer f.Close()
    
    buffer := make([]byte, 512)
    _, err = f.Read(buffer)
    if err != nil {
        return err
    }
    
    contentType := http.DetectContentType(buffer)
    if !AllowedArchiveTypes[contentType] {
        // Allow HTML/text for archived pages
        if contentType != "text/html; charset=utf-8" && contentType != "text/plain; charset=utf-8" {
            return ErrInvalidType
        }
    }
    
    return nil
}

func ValidateThumbnail(file *multipart.FileHeader) error {
    if file.Size > MaxThumbnailSize {
        return ErrFileTooLarge
    }
    
    f, err := file.Open()
    if err != nil {
        return err
    }
    defer f.Close()
    
    buffer := make([]byte, 512)
    _, err = f.Read(buffer)
    if err != nil {
        return err
    }
    
    contentType := http.DetectContentType(buffer)
    if !AllowedThumbnailTypes[contentType] {
        return ErrInvalidType
    }
    
    return nil
}
```

---

### 10. Password Security

**Current Implementation:** ✅ **Strong**

```go
// internal/auth/password.go
const bcryptCost = 12  // Good balance
const MinPasswordLength = 8

func ValidatePassword(password string) error {
    // Requires 3 of 4: uppercase, lowercase, digit, special
}
```

**Assessment:** Password security is well-implemented.

**Minor Recommendations:**
- Add password breach checking via haveibeenpwned API (optional)
- Add password strength meter in frontend (UX improvement)

---

## Vulnerability Summary

### Critical (Fix Immediately)

| ID | Vulnerability | Severity | Effort |
|----|---------------|----------|--------|
| SEC-001 | CORS allows all origins | Critical | 1h |
| SEC-002 | No CSRF protection | Critical | 2h |
| SEC-003 | Cookie flags missing (HttpOnly, Secure) | Critical | 0.5h |

### High Priority (Fix Within Sprint)

| ID | Vulnerability | Severity | Effort |
|----|---------------|----------|--------|
| SEC-004 | No rate limiting | High | 2h |
| SEC-005 | Incomplete security headers | High | 1h |
| SEC-006 | UUID validation weak | High | 0.5h |

### Medium Priority (Fix Soon)

| ID | Vulnerability | Severity | Effort |
|----|---------------|----------|--------|
| SEC-007 | File upload content validation | Medium | 2h |
| SEC-008 | Session not regenerated | Medium | 1h |
| SEC-009 | No audit logging | Medium | 3h |

### Low Priority (Improve Over Time)

| ID | Vulnerability | Severity | Effort |
|----|---------------|----------|--------|
| SEC-010 | CSP varies by endpoint | Low | 2h |
| SEC-011 | No HSTS header | Low | 0.5h |
| SEC-012 | No security.txt | Low | 0.5h |

---

## Implementation Plan

### Phase 1: Critical Fixes (1 Day)

**Priority:** Complete before any production deployment

```go
// 1. Fix cookie security (handler/auth.go)
c.SetCookie("session", sessionID, 
    int(h.sessionTTL.Seconds()), 
    "/", "", true, true)  // Secure=true, HttpOnly=true

// 2. Fix CORS (middleware/common.go)
func CORS(allowedOrigins []string) gin.HandlerFunc {
    // Validate origin against allowlist
}

// 3. Add CSRF protection (new file)
r.Use(middleware.CSRF())
```

**Files to Modify:**
- `internal/handler/auth.go` - Cookie security
- `internal/middleware/common.go` - CORS
- `internal/middleware/csrf.go` - New file
- `cmd/server/main.go` - Enable CSRF

### Phase 2: Rate Limiting & Headers (1 Day)

**Priority:** Add defense-in-depth

```go
// 1. Add rate limiting (new file)
auth.POST("/login", middleware.AuthRateLimit(), authHandler.Login)

// 2. Add security headers (new file)
r.Use(middleware.SecurityHeaders())

// 3. Improve UUID validation
func RequireUUIDParam(paramName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Param(paramName)
        if _, err := uuid.Parse(id); err != nil {
            // Reject
        }
    }
}
```

**Files to Create/Modify:**
- `internal/middleware/rate_limit.go` - New file
- `internal/middleware/security_headers.go` - New file
- `internal/middleware/uuid.go` - Improve validation

### Phase 3: File Upload Security (1 Day)

**Priority:** Prevent malicious file uploads

```go
// Add content type validation
func ValidateArchive(file *multipart.FileHeader) error { ... }
func ValidateThumbnail(file *multipart.FileHeader) error { ... }
```

**Files to Create/Modify:**
- `internal/service/file_validation.go` - New file
- `internal/handler/file.go` - Add validation calls

### Phase 4: Session Improvements (1 Day)

**Priority:** Enhance session security

```go
// 1. Session regeneration
func (h *AuthHandler) Login(c *gin.Context) {
    // After successful login, regenerate session ID
    newSessionID := uuid.New().String()
    // ...
}

// 2. Sliding expiration
func (m *AuthMiddleware) Authenticate() gin.HandlerFunc {
    // If session is close to expiring, extend it
}

// 3. Audit logging
func AuditLog(action, userID, resource string) {
    // Log security-relevant actions
}
```

---

## Security Checklist

### Authentication & Sessions
- [ ] Set `Secure` flag on session cookie
- [ ] Set `HttpOnly` flag on session cookie
- [ ] Set `SameSite=Strict` on session cookie
- [ ] Regenerate session ID after login
- [ ] Invalidate old sessions on password change
- [ ] Implement session timeout with cleanup

### Authorization
- [x] User ownership validation on all resources
- [ ] Add audit logging for sensitive operations
- [ ] Consider rate limiting per-user (not just per-IP)

### Input Validation
- [ ] Use `uuid.Parse()` for UUID validation
- [ ] Validate all URL inputs with allowlist
- [ ] Add content-type validation for uploads
- [ ] Set maximum file sizes for uploads

### CORS
- [ ] Replace `*` with explicit origin allowlist
- [ ] Configure `Access-Control-Allow-Credentials` properly
- [ ] Remove `Authorization` from allowed headers (use cookies)

### CSRF
- [ ] Implement CSRF token generation
- [ ] Add CSRF token validation middleware
- [ ] Exclude safe methods (GET, HEAD, OPTIONS)

### Rate Limiting
- [ ] Add rate limiter for auth endpoints (5/min)
- [ ] Add rate limiter for API endpoints (100/min)
- [ ] Add rate limiter for archive endpoints (10/min)

### Security Headers
- [ ] Add `X-Content-Type-Options: nosniff`
- [ ] Add `X-Frame-Options: DENY`
- [ ] Add `Referrer-Policy: strict-origin-when-cross-origin`
- [ ] Add `Permissions-Policy` to disable features
- [ ] Add `Content-Security-Policy` (base policy)
- [ ] Add `Strict-Transport-Security` (HTTPS only)

### File Handling
- [x] Path traversal protection
- [ ] Content type validation
- [ ] File size limits
- [ ] Magic byte verification

---

## Testing Recommendations

### Security Test Cases

```go
// internal/middleware/security_test.go

func TestCSRFProtection(t *testing.T) {
    // Test that POST without CSRF token fails
    // Test that POST with valid CSRF token succeeds
    // Test that GET requests don't require CSRF
}

func TestRateLimiting(t *testing.T) {
    // Test that exceeding rate limit returns 429
    // Test that rate limit is per-IP
}

func TestCookieSecurity(t *testing.T) {
    // Test that session cookie has Secure flag
    // Test that session cookie has HttpOnly flag
    // Test that session cookie has SameSite
}

func TestCORS(t *testing.T) {
    // Test that disallowed origins are rejected
    // Test that allowed origins are accepted
}

func TestUUIDValidation(t *testing.T) {
    // Test that invalid UUIDs are rejected
    // Test that path traversal in UUID is blocked
}

func TestPathTraversal(t *testing.T) {
    // Test all known traversal payloads
    // Test URL-encoded variants
    // Test double-encoded variants
}
```

### Penetration Testing Checklist

- [ ] OWASP Top 10 assessment
- [ ] Session management testing
- [ ] Authentication bypass attempts
- [ ] Path traversal fuzzing
- [ ] XSS payload testing
- [ ] CSRF token validation
- [ ] Rate limit bypass attempts

---

## Configuration Reference

### Environment Variables

```bash
# Security configuration
CORS_ALLOWED_ORIGINS=https://example.com,https://app.example.com
SESSION_SECURE=true
SESSION_HTTP_ONLY=true
SESSION_SAME_SITE=Strict
SESSION_TTL_HOURS=168  # 7 days
RATE_LIMIT_AUTH=5      # 5 requests per minute for auth
RATE_LIMIT_API=100     # 100 requests per minute for API
HTTPS_ENABLED=true     # Enable HSTS
```

### Production Checklist

Before deploying to production:

1. [ ] Set `CORS_ALLOWED_ORIGINS` to production domains
2. [ ] Set `SESSION_SECURE=true`
3. [ ] Set `HTTPS_ENABLED=true` (enables HSTS)
4. [ ] Configure reverse proxy with TLS
5. [ ] Enable rate limiting
6. [ ] Review security headers with `curl -I`
7. [ ] Run security test suite
8. [ ] Perform penetration testing

---

## Appendix A: Security Headers Reference

### Complete Headers Configuration

```go
// For API endpoints
func APIHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
        c.Header("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
        c.Next()
    }
}

// For archive file serving (more permissive for archived content)
func ArchiveHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("Cross-Origin-Resource-Policy", "same-origin")
        c.Header("Cross-Origin-Opener-Policy", "same-origin")
        c.Header("Referrer-Policy", "no-referrer")
        c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; media-src 'self' data:; script-src 'self' 'unsafe-inline'; frame-ancestors 'self'; base-uri 'none'; connect-src 'none';")
        c.Next()
    }
}

// For frontend (SvelteKit)
func FrontendHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "SAMEORIGIN")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'self';")
        c.Next()
    }
}
```

---

## Appendix B: OWASP Top 10 Mapping

| OWASP Category | Status | Notes |
|----------------|--------|-------|
| A01:2021 Broken Access Control | ✅ Good | User ownership validated |
| A02:2021 Cryptographic Failures | ⚠️ Partial | Passwords secure, cookies not |
| A03:2021 Injection | ✅ Good | GORM parameterized queries |
| A04:2021 Insecure Design | ⚠️ Partial | Missing CSRF, rate limiting |
| A05:2021 Security Misconfiguration | ⚠️ Partial | CORS misconfigured |
| A06:2021 Vulnerable Components | TBD | Run `go list -m -u all` |
| A07:2021 Identification & Authentication | ⚠️ Partial | Session management weak |
| A08:2021 Software & Data Integrity | ✅ Good | Go modules verified |
| A09:2021 Security Logging & Monitoring | ❌ Missing | No audit logging |
| A10:2021 Server-Side Request Forgery | N/A | No external URL fetching |

---

*End of ADR-005*