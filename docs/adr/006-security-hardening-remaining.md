# ADR-006: Security Hardening - Remaining Implementation

**Status:** Proposed  
**Date:** 2026-03-09  
**Decision Makers:** @bumblebee, @jarvis  
**Supersedes:** N/A  
**Related ADRs:** ADR-004 (Error Handling), ADR-005 (Security Architecture Review)

---

## Executive Summary

This ADR addresses the remaining security hardening tasks identified during the implementation review of ADR-005. It provides detailed specifications for:
1. CORS configuration
2. Auth rate limiting
3. CSRF protection
4. UUID validation improvement
5. File upload security
6. Session regeneration
7. Audit logging

---

## Context

During the security implementation review, several items were identified as incomplete or needing additional configuration:

| Item | Status | Priority |
|------|--------|----------|
| CORS configuration | ⚠️ Code exists, needs config | Critical |
| Auth rate limiting | ⚠️ Global only, missing auth | Critical |
| CSRF tokens | ❌ Not implemented | High |
| UUID validation | ⚠️ Format check only | Medium |
| File upload validation | ❌ Not implemented | Medium |
| Session regeneration | ❌ Not implemented | Medium |
| Audit logging | ❌ Not implemented | Low |

---

## Decision

### 1. CORS Configuration (Critical)

**Current State:**
```go
r.Use(middleware.StrictCORS([]string{}))  // Empty array
```

**Problem:** Empty allowed origins array will reject all cross-origin requests.

**Solution:** Configure allowed origins via environment variable with sensible defaults.

**Implementation:**

```go
// cmd/server/main.go

import (
    "os"
    "strings"
)

func setupRouter(...) *gin.Engine {
    r := gin.Default()
    
    // Configure CORS from environment
    corsOrigins := getCORSOrigins()
    r.Use(middleware.StrictCORS(corsOrigins))
    
    // ... rest of router setup
}

// getCORSOrigins returns allowed CORS origins from environment
func getCORSOrigins() []string {
    // Production: CORS_ALLOWED_ORIGINS=https://app.example.com,https://example.com
    // Development: CORS_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
    envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
    if envOrigins != "" {
        return strings.Split(envOrigins, ",")
    }
    
    // Default: Allow same-origin only (secure by default)
    // In development, you might want to allow localhost
    if os.Getenv("GO_ENV") == "development" {
        return []string{
            "http://localhost:5173",  // Vite dev server
            "http://localhost:3000",  // Alternative dev port
            "http://localhost:8080",  // Backend server
        }
    }
    
    // Production default: no cross-origin allowed
    return []string{}
}
```

**Environment Configuration:**

```bash
# Development (.env.development)
GO_ENV=development
CORS_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000

# Production (.env.production)
GO_ENV=production
CORS_ALLOWED_ORIGINS=https://yourdomain.com,https://app.yourdomain.com
```

---

### 2. Auth Rate Limiting (Critical)

**Current State:**
```go
r.Use(middleware.RateLimit(100, 1*time.Minute))  // Global 100/min
// Auth endpoints have NO additional rate limiting
auth.POST("/register", authHandler.Register)
auth.POST("/login", authHandler.Login)
```

**Problem:** Authentication endpoints are vulnerable to brute force attacks.

**Solution:** Apply stricter rate limiting to authentication endpoints.

**Implementation:**

```go
// cmd/server/main.go

func setupRouter(...) *gin.Engine {
    r := gin.Default()
    
    // Global rate limit: 100 requests per minute
    globalLimiter := middleware.RateLimit(100, 1*time.Minute)
    r.Use(globalLimiter)
    
    // Auth rate limit: 5 requests per minute per IP
    // This is stricter to prevent brute force
    authLimiter := middleware.RateLimit(5, 1*time.Minute)
    
    // Auth rate limit: 3 registrations per hour per IP
    registerLimiter := middleware.RateLimit(3, 1*time.Hour)
    
    // Public auth routes with rate limiting
    auth := api.Group("/auth")
    {
        auth.POST("/register", authLimiter, registerLimiter, authHandler.Register)
        auth.POST("/login", authLimiter, authHandler.Login)
    }
    
    // ... rest of router setup
}
```

**Rate Limit Configuration:**

```go
// internal/config/rate_limits.go

package config

import "time"

type RateLimitConfig struct {
    Name        string
    Limit       int
    Window      time.Duration
    KeyFunc     func(c *gin.Context) string
}

var DefaultRateLimits = map[string]RateLimitConfig{
    "global": {
        Name:   "global",
        Limit:  100,
        Window: 1 * time.Minute,
        KeyFunc: func(c *gin.Context) string {
            return c.ClientIP()
        },
    },
    "auth": {
        Name:   "auth",
        Limit:  5,
        Window: 1 * time.Minute,
        KeyFunc: func(c *gin.Context) string {
            // Rate limit by IP + email to prevent distributed attacks
            email := c.GetString("login_email")
            if email == "" {
                return c.ClientIP()
            }
            return c.ClientIP() + ":" + email
        },
    },
    "register": {
        Name:   "register",
        Limit:  3,
        Window: 1 * time.Hour,
        KeyFunc: func(c *gin.Context) string {
            return c.ClientIP()
        },
    },
}
```

---

### 3. CSRF Protection (High Priority)

**Current State:** SameSite=Strict cookie attribute provides partial CSRF protection.

**Problem:** SameSite doesn't protect against:
- Same-site attacks
- Older browsers
- Edge cases where SameSite can be bypassed

**Solution:** Implement CSRF token validation for state-changing requests.

**Implementation:**

```go
// internal/middleware/csrf.go

package middleware

import (
    "crypto/subtle"
    "net/http"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

const (
    // CSRFTokenHeader is the header name for CSRF token
    CSRFTokenHeader = "X-CSRF-Token"
    // CSRFTokenCookie is the cookie name for CSRF token
    CSRFTokenCookie = "csrf_token"
    // CSRFTokenContext is the context key for CSRF token
    CSRFTokenContext = "csrf_token"
)

// CSRFConfig holds CSRF middleware configuration
type CSRFConfig struct {
    // TokenLength is the length of generated tokens
    TokenLength int
    // Secure cookie setting
    Secure bool
    // Path for cookie
    Path string
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() CSRFConfig {
    return CSRFConfig{
        TokenLength: 32,
        Secure:      true,
        Path:        "/",
    }
}

// CSRF returns a CSRF protection middleware
func CSRF(config ...CSRFConfig) gin.HandlerFunc {
    cfg := DefaultCSRFConfig()
    if len(config) > 0 {
        cfg = config[0]
    }
    
    return func(c *gin.Context) {
        // Skip safe methods
        if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" || c.Request.Method == "TRACE" {
            c.Next()
            return
        }
        
        // Get token from cookie
        cookieToken, err := c.Cookie(CSRFTokenCookie)
        if err != nil || cookieToken == "" {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "type":   "about:blank",
                "title":  "Forbidden",
                "status": http.StatusForbidden,
                "detail": "CSRF token missing",
                "code":   "CSRF_MISSING",
            })
            return
        }
        
        // Get token from header
        headerToken := c.GetHeader(CSRFTokenHeader)
        if headerToken == "" {
            // Also check form field for form submissions
            headerToken = c.PostForm("_csrf")
        }
        
        if headerToken == "" {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "type":   "about:blank",
                "title":  "Forbidden",
                "status": http.StatusForbidden,
                "detail": "CSRF token required",
                "code":   "CSRF_REQUIRED",
            })
            return
        }
        
        // Constant-time comparison to prevent timing attacks
        if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "type":   "about:blank",
                "title":  "Forbidden",
                "status": http.StatusForbidden,
                "detail": "CSRF token invalid",
                "code":   "CSRF_INVALID",
            })
            return
        }
        
        c.Next()
    }
}

// GenerateCSRFToken generates a new CSRF token
func GenerateCSRFToken() string {
    return uuid.New().String()
}

// SetCSRFToken sets a new CSRF token on the response
func SetCSRFToken(c *gin.Context, secure bool) string {
    token := GenerateCSRFToken()
    
    // Set cookie with security attributes
    c.SetCookie(CSRFTokenCookie, token, 86400, "/", "", secure, true)
    
    // Store in context for handlers to access
    c.Set(CSRFTokenContext, token)
    
    return token
}

// GetCSRFToken retrieves the CSRF token from context
func GetCSRFToken(c *gin.Context) string {
    if token, exists := c.Get(CSRFTokenContext); exists {
        return token.(string)
    }
    return ""
}

// CSRFTokenEndpoint returns a handler that provides CSRF tokens
func CSRFTokenEndpoint(secure bool) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := SetCSRFToken(c, secure)
        c.JSON(http.StatusOK, gin.H{
            "csrf_token": token,
        })
    }
}
```

**Integration:**

```go
// cmd/server/main.go

func setupRouter(...) *gin.Engine {
    r := gin.Default()
    
    // Security headers
    r.Use(middleware.SecurityHeaders())
    
    // CSRF protection for state-changing requests
    // Note: Exclude public auth routes (login/register) from CSRF since they're API-only
    r.Use(middleware.CSRF())
    
    // Rate limiting: 100 requests per minute
    r.Use(middleware.RateLimit(100, 1*time.Minute))
    
    // ... rest of setup
    
    // CSRF token endpoint for clients
    r.GET("/api/v1/csrf-token", middleware.CSRFTokenEndpoint(true))
    
    // ... routes
}
```

**Client Integration:**

```typescript
// frontend/src/lib/api/csrf.ts

let csrfToken: string | null = null;

export async function getCSRFToken(): Promise<string> {
    if (csrfToken) return csrfToken;
    
    const response = await fetch('/api/v1/csrf-token');
    const data = await response.json();
    csrfToken = data.csrf_token;
    
    return csrfToken;
}

export async function fetchWithCSRF(url: string, options: RequestInit = {}): Promise<Response> {
    const token = await getCSRFToken();
    
    const headers = new Headers(options.headers || {});
    headers.set('X-CSRF-Token', token);
    
    return fetch(url, {
        ...options,
        headers,
        credentials: 'include',  // Include cookies
    });
}
```

---

### 4. UUID Validation Improvement (Medium Priority)

**Current State:**
```go
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
```

**Problem:** Only checks format, doesn't validate it's a valid UUID.

**Solution:** Use `uuid.Parse()` for proper validation.

**Implementation:**

```go
// internal/middleware/uuid.go

package middleware

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

// RequireUUIDParam returns a middleware that validates a path parameter as a UUID.
// Uses uuid.Parse() for proper UUID validation.
func RequireUUIDParam(paramName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Param(paramName)
        
        // Validate UUID using uuid.Parse
        if _, err := uuid.Parse(id); err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "type":   "about:blank",
                "title":  "Bad Request",
                "status": http.StatusBadRequest,
                "detail": "invalid " + paramName + ": must be a valid UUID",
                "code":   "INVALID_UUID",
            })
            return
        }
        
        c.Next()
    }
}

// RequireUUIDQuery validates a query parameter as a UUID
func RequireUUIDQuery(paramName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.Query(paramName)
        
        if id == "" {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "type":   "about:blank",
                "title":  "Bad Request",
                "status": http.StatusBadRequest,
                "detail": "missing " + paramName + " query parameter",
                "code":   "MISSING_PARAMETER",
            })
            return
        }
        
        if _, err := uuid.Parse(id); err != nil {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "type":   "about:blank",
                "title":  "Bad Request",
                "status": http.StatusBadRequest,
                "detail": "invalid " + paramName + ": must be a valid UUID",
                "code":   "INVALID_UUID",
            })
            return
        }
        
        c.Next()
    }
}

// IsValidUUID checks if a string is a valid UUID
func IsValidUUID(id string) bool {
    _, err := uuid.Parse(id)
    return err == nil
}
```

---

### 5. File Upload Security (Medium Priority)

**Current State:** Path traversal protection exists, but no content validation.

**Problem:** Files can be uploaded with malicious content or incorrect types.

**Solution:** Implement content type validation, size limits, and magic byte verification.

**Implementation:**

```go
// internal/service/file_validation.go

package service

import (
    "bytes"
    "errors"
    "mime/multipart"
    "net/http"
    "path/filepath"
)

var (
    ErrFileTooLarge    = errors.New("file too large")
    ErrInvalidType     = errors.New("invalid file type")
    ErrInvalidContent  = errors.New("file content does not match extension")
)

const (
    MaxArchiveSize   = 100 * 1024 * 1024 // 100 MB
    MaxThumbnailSize = 10 * 1024 * 1024  // 10 MB
    MaxImageSize     = 10 * 1024 * 1024  // 10 MB
)

// AllowedMimeTypes defines allowed MIME types by category
var AllowedMimeTypes = map[string][]string{
    "image": {
        "image/jpeg",
        "image/png",
        "image/gif",
        "image/webp",
    },
    "archive": {
        "application/zip",
        "application/x-zip-compressed",
        "application/gzip",
        "application/x-gzip",
    },
    "document": {
        "text/html",
        "text/plain",
        "application/pdf",
    },
}

// MagicBytes maps file signatures to MIME types
var MagicBytes = map[string]struct {
    Signature []byte
    MIMEType  string
}{
    "jpeg": {[]byte{0xFF, 0xD8, 0xFF}, "image/jpeg"},
    "png":  {[]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
    "gif":  {[]byte{0x47, 0x49, 0x46, 0x38}, "image/gif"},
    "webp": {[]byte{0x52, 0x49, 0x46, 0x46}, "image/webp"}, // RIFF header, need to check WEBP
    "pdf":  {[]byte{0x25, 0x50, 0x44, 0x46}, "application/pdf"},
    "zip":  {[]byte{0x50, 0x4B, 0x03, 0x04}, "application/zip"},
    "gzip": {[]byte{0x1F, 0x8B}, "application/gzip"},
}

// FileValidator validates uploaded files
type FileValidator struct {
    maxSize     int64
    allowedMIME []string
}

// NewImageValidator creates a validator for image uploads
func NewImageValidator() *FileValidator {
    return &FileValidator{
        maxSize:     MaxImageSize,
        allowedMIME: AllowedMimeTypes["image"],
    }
}

// NewArchiveValidator creates a validator for archive uploads
func NewArchiveValidator() *FileValidator {
    allowed := append(AllowedMimeTypes["archive"], AllowedMimeTypes["document"]...)
    return &FileValidator{
        maxSize:     MaxArchiveSize,
        allowedMIME: allowed,
    }
}

// Validate checks if the file meets all requirements
func (v *FileValidator) Validate(file *multipart.FileHeader) error {
    // Check file size
    if file.Size > v.maxSize {
        return ErrFileTooLarge
    }
    
    // Open file to read content
    f, err := file.Open()
    if err != nil {
        return err
    }
    defer f.Close()
    
    // Read first 512 bytes for content detection
    buffer := make([]byte, 512)
    n, err := f.Read(buffer)
    if err != nil && err != io.EOF {
        return err
    }
    buffer = buffer[:n]
    
    // Detect content type
    detectedType := http.DetectContentType(buffer)
    
    // Check against allowed MIME types
    if !v.isAllowedMIME(detectedType) {
        return ErrInvalidType
    }
    
    // Verify magic bytes for extra security
    if err := v.verifyMagicBytes(buffer, detectedType); err != nil {
        return err
    }
    
    return nil
}

// isAllowedMIME checks if the MIME type is allowed
func (v *FileValidator) isAllowedMIME(mimeType string) bool {
    for _, allowed := range v.allowedMIME {
        if allowed == mimeType || 
           (strings.HasPrefix(mimeType, "text/") && allowed == "text/plain") ||
           (strings.HasPrefix(mimeType, "image/") && allowed == "image/*") {
            return true
        }
    }
    return false
}

// verifyMagicBytes verifies file signature matches content type
func (v *FileValidator) verifyMagicBytes(buffer []byte, detectedType string) error {
    // For images, verify magic bytes
    if strings.HasPrefix(detectedType, "image/") {
        for name, sig := range MagicBytes {
            if bytes.HasPrefix(buffer, sig.Signature) {
                // Special check for WebP (RIFF....WEBP)
                if name == "webp" && len(buffer) >= 12 {
                    if string(buffer[8:12]) != "WEBP" {
                        continue
                    }
                }
                return nil
            }
        }
        // If we detected an image but magic bytes don't match, be suspicious
        return ErrInvalidContent
    }
    
    return nil
}

// SanitizeFilename removes dangerous characters from filename
func SanitizeFilename(filename string) string {
    // Remove path separators
    filename = filepath.Base(filename)
    
    // Remove null bytes
    filename = strings.ReplaceAll(filename, "\x00", "")
    
    // Remove control characters
    var result strings.Builder
    for _, r := range filename {
        if !unicode.IsControl(r) {
            result.WriteRune(r)
        }
    }
    
    return result.String()
}
```

**Integration in Handler:**

```go
// internal/handler/file.go

func (h *FileHandler) UploadThumbnail(c *gin.Context) {
    // ... authentication checks ...
    
    file, err := c.FormFile("file")
    if err != nil {
        SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "no file provided"))
        return
    }
    
    // Validate file
    validator := service.NewImageValidator()
    if err := validator.Validate(file); err != nil {
        if errors.Is(err, service.ErrFileTooLarge) {
            SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "file too large (max 10MB)"))
        } else if errors.Is(err, service.ErrInvalidType) {
            SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "invalid file type (allowed: JPEG, PNG, GIF, WebP)"))
        } else if errors.Is(err, service.ErrInvalidContent) {
            SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "file content does not match extension"))
        } else {
            SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "file validation failed", err))
        }
        return
    }
    
    // Sanitize filename
    filename := service.SanitizeFilename(file.Filename)
    
    // ... rest of upload logic ...
}
```

---

### 6. Session Regeneration (Medium Priority)

**Current State:** Session ID remains the same after login.

**Problem:** Session fixation attacks if session ID is leaked before authentication.

**Solution:** Regenerate session ID after successful authentication.

**Implementation:**

```go
// internal/handler/auth.go

func (h *AuthHandler) Login(c *gin.Context) {
    // ... validation and authentication ...
    
    // Check if user is active
    if !user.IsActive {
        SendError(c, apperrors.NewForbiddenError(apperrors.CodeForbidden, "account is disabled"))
        return
    }
    
    // IMPORTANT: Regenerate session ID after successful authentication
    // This prevents session fixation attacks
    oldSessionID, _ := c.Cookie("session")
    if oldSessionID != "" {
        // Delete old session if exists
        h.sessionRepo.Delete(oldSessionID)
    }
    
    // Create new session with fresh ID
    session := &model.Session{
        ID:        uuid.New().String(),
        UserID:    user.ID,
        ExpiresAt: time.Now().Add(h.sessionTTL),
        CreatedAt: time.Now(),
    }
    
    if err := h.sessionRepo.Create(session); err != nil {
        SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to create session", err))
        return
    }
    
    // Set session cookie
    h.setSessionCookie(c, session.ID)
    
    c.JSON(http.StatusOK, AuthResponse{
        User: UserResponse{
            ID:          user.ID,
            Email:       user.Email,
            DisplayName: user.DisplayName,
            CreatedAt:   user.CreatedAt,
        },
    })
}

func (h *AuthHandler) Register(c *gin.Context) {
    // ... validation and user creation ...
    
    // After successful registration, create a fresh session
    // (don't reuse any pre-existing session ID)
    session := &model.Session{
        ID:        uuid.New().String(),
        UserID:    user.ID,
        ExpiresAt: time.Now().Add(h.sessionTTL),
        CreatedAt: time.Now(),
    }
    
    if err := h.sessionRepo.Create(session); err != nil {
        SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to create session", err))
        return
    }
    
    h.setSessionCookie(c, session.ID)
    
    // ... response ...
}
```

---

### 7. Audit Logging (Low Priority)

**Current State:** Basic request logging only, no security event logging.

**Problem:** No visibility into security-relevant events for incident response.

**Solution:** Implement structured audit logging for security events.

**Implementation:**

```go
// internal/audit/audit.go

package audit

import (
    "context"
    "encoding/json"
    "log"
    "os"
    "time"
)

// Event represents an audit event
type Event struct {
    Timestamp   time.Time              `json:"timestamp"`
    EventType   string                 `json:"event_type"`
    UserID      string                 `json:"user_id,omitempty"`
    IPAddress   string                 `json:"ip_address"`
    UserAgent   string                 `json:"user_agent"`
    Resource    string                 `json:"resource,omitempty"`
    Action      string                 `json:"action"`
    Success     bool                   `json:"success"`
    Details     map[string]interface{} `json:"details,omitempty"`
    Error       string                 `json:"error,omitempty"`
}

// Event types
const (
    EventTypeAuth       = "auth"
    EventTypeSession    = "session"
    EventTypeEntry      = "entry"
    EventTypeArchive    = "archive"
    EventTypeSecurity   = "security"
)

// Actions
const (
    ActionLogin         = "login"
    ActionLogout        = "logout"
    ActionRegister      = "register"
    ActionPasswordChange = "password_change"
    ActionCreate        = "create"
    ActionUpdate        = "update"
    ActionDelete        = "delete"
    ActionView          = "view"
    ActionExport        = "export"
)

// Logger handles audit logging
type Logger struct {
    logger *log.Logger
}

// NewLogger creates a new audit logger
func NewLogger() *Logger {
    return &Logger{
        logger: log.New(os.Stdout, "[AUDIT] ", log.LstdFlags),
    }
}

// Log writes an audit event
func (l *Logger) Log(ctx context.Context, event Event) {
    // Ensure timestamp is set
    if event.Timestamp.IsZero() {
        event.Timestamp = time.Now()
    }
    
    // Convert to JSON
    data, err := json.Marshal(event)
    if err != nil {
        l.logger.Printf("ERROR marshaling audit event: %v", err)
        return
    }
    
    // Write to stdout (can be redirected to file or log aggregator)
    l.logger.Println(string(data))
}

// Convenience functions

func (l *Logger) LogLogin(ctx context.Context, userID, ipAddress, userAgent string, success bool, err string) {
    l.Log(ctx, Event{
        EventType: EventTypeAuth,
        Action:    ActionLogin,
        UserID:    userID,
        IPAddress: ipAddress,
        UserAgent: userAgent,
        Success:   success,
        Error:     err,
    })
}

func (l *Logger) LogLogout(ctx context.Context, userID, ipAddress string) {
    l.Log(ctx, Event{
        EventType: EventTypeAuth,
        Action:    ActionLogout,
        UserID:    userID,
        IPAddress: ipAddress,
        Success:   true,
    })
}

func (l *Logger) LogRegister(ctx context.Context, userID, ipAddress, userAgent string, success bool, err string) {
    l.Log(ctx, Event{
        EventType: EventTypeAuth,
        Action:    ActionRegister,
        UserID:    userID,
        IPAddress: ipAddress,
        UserAgent: userAgent,
        Success:   success,
        Error:     err,
    })
}

func (l *Logger) LogEntryAction(ctx context.Context, userID, action, entryID string, success bool, details map[string]interface{}) {
    l.Log(ctx, Event{
        EventType: EventTypeEntry,
        Action:    action,
        UserID:    userID,
        Resource:  entryID,
        Success:   success,
        Details:   details,
    })
}

func (l *Logger) LogSecurityEvent(ctx context.Context, action, ipAddress, userAgent string, details map[string]interface{}) {
    l.Log(ctx, Event{
        EventType: EventTypeSecurity,
        Action:    action,
        IPAddress: ipAddress,
        UserAgent: userAgent,
        Success:   false,
        Details:   details,
    })
}
```

**Integration in Auth Handler:**

```go
// internal/handler/auth.go

type AuthHandler struct {
    userRepo    *repository.UserRepository
    sessionRepo *repository.SessionRepository
    sessionTTL  time.Duration
    audit       *audit.Logger  // Add audit logger
}

func NewAuthHandler(userRepo *repository.UserRepository, sessionRepo *repository.SessionRepository) *AuthHandler {
    return &AuthHandler{
        userRepo:    userRepo,
        sessionRepo: sessionRepo,
        sessionTTL:  7 * 24 * time.Hour,
        audit:       audit.NewLogger(),
    }
}

func (h *AuthHandler) Login(c *gin.Context) {
    ipAddress := c.ClientIP()
    userAgent := c.GetHeader("User-Agent")
    
    // ... validation ...
    
    // On success
    h.audit.LogLogin(c.Request.Context(), user.ID, ipAddress, userAgent, true, "")
    
    // On failure
    h.audit.LogLogin(c.Request.Context(), "", ipAddress, userAgent, false, "invalid credentials")
}
```

---

## Implementation Order

### Phase 1: Critical (Implement Immediately)

1. **CORS Configuration** (15 min)
   - Update `cmd/server/main.go` to read from environment
   - Add to `.env.example`

2. **Auth Rate Limiting** (30 min)
   - Create auth-specific rate limiters
   - Apply to login and register endpoints

### Phase 2: High Priority (Implement Soon)

3. **CSRF Protection** (2 hours)
   - Create `internal/middleware/csrf.go`
   - Add CSRF token endpoint
   - Update frontend to include CSRF token

### Phase 3: Medium Priority (Implement When Possible)

4. **UUID Validation** (30 min)
   - Update `internal/middleware/uuid.go`
   - Use `uuid.Parse()` for validation

5. **File Upload Security** (2 hours)
   - Create `internal/service/file_validation.go`
   - Integrate into file handlers

6. **Session Regeneration** (30 min)
   - Update auth handler to regenerate session ID

### Phase 4: Low Priority (Future Enhancement)

7. **Audit Logging** (3 hours)
   - Create `internal/audit/audit.go`
   - Integrate into auth and entry handlers
   - Set up log aggregation

---

## Testing Requirements

### Unit Tests

```go
// internal/middleware/csrf_test.go

func TestCSRFProtection(t *testing.T) {
    tests := []struct {
        name         string
        method       string
        cookie       string
        header       string
        expectStatus int
    }{
        {"GET without token", "GET", "", "", 200},
        {"POST without token", "POST", "", "", 403},
        {"POST with matching token", "POST", "valid-token", "valid-token", 200},
        {"POST with mismatched token", "POST", "token1", "token2", 403},
    }
    // ... test implementation
}

// internal/service/file_validation_test.go

func TestImageValidation(t *testing.T) {
    tests := []struct {
        name       string
        filename   string
        content    []byte
        expectError error
    }{
        {"valid JPEG", "test.jpg", validJPEG, nil},
        {"invalid extension", "test.jpg", maliciousContent, ErrInvalidContent},
        {"too large", "test.jpg", largeContent, ErrFileTooLarge},
    }
    // ... test implementation
}
```

### Integration Tests

```go
// cmd/server/main_test.go

func TestCORSConfiguration(t *testing.T) {
    tests := []struct {
        origin      string
        expectAllow bool
    }{
        {"http://localhost:5173", true},
        {"https://malicious.com", false},
    }
    // ... test implementation
}

func TestAuthRateLimiting(t *testing.T) {
    // Test that 6th login attempt within 1 minute returns 429
}
```

---

## Environment Configuration Reference

```bash
# .env.example

# CORS Configuration
CORS_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
# For production:
# CORS_ALLOWED_ORIGINS=https://yourdomain.com,https://app.yourdomain.com

# Rate Limiting
RATE_LIMIT_GLOBAL=100        # requests per minute
RATE_LIMIT_AUTH=5            # auth requests per minute
RATE_LIMIT_REGISTER=3        # registrations per hour

# Session
SESSION_SECURE=true          # Set to false for development without HTTPS
SESSION_HTTP_ONLY=true
SESSION_SAME_SITE=strict    # strict, lax, or none

# CSRF
CSRF_ENABLED=true            # Set to false for API-only deployments

# Audit Logging
AUDIT_LOG_ENABLED=true
AUDIT_LOG_FORMAT=json        # json or text
```

---

## Monitoring & Alerting

### Recommended Alerts

1. **Rate Limit Exceeded**
   - Alert when rate limit exceeded > 10 times per minute
   - Possible brute force attack

2. **CSRF Token Failures**
   - Alert when CSRF failures > 5 times per minute
   - Possible CSRF attack

3. **Failed Login Attempts**
   - Alert when failed logins > 10 from same IP
   - Possible credential stuffing

4. **File Upload Failures**
   - Alert when file validation fails > 20 times per hour
   - Possible malicious file upload attempts

---

## Rollout Plan

### Step 1: Deploy CORS and Rate Limiting (Low Risk)
- Configure CORS from environment
- Add auth rate limiting
- Monitor for issues

### Step 2: Deploy CSRF Protection (Medium Risk)
- Add CSRF middleware
- Add CSRF token endpoint
- Update frontend to include tokens
- Test thoroughly

### Step 3: Deploy File Validation (Medium Risk)
- Add file validation service
- Integrate into upload handlers
- Test with various file types

### Step 4: Deploy Remaining Items (Low Risk)
- UUID validation improvement
- Session regeneration
- Audit logging

---

*End of ADR-006*