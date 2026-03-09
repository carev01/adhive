# ADR-004: Error Handling & Recovery Patterns

**Status:** Proposed  
**Date:** 2026-03-09  
**Decision Makers:** @bumblebee, @jarvis  
**Related ADRs:** ADR-002 (Performance Optimization)

---

## Executive Summary

This ADR defines error handling standards for AdHive, establishing consistent patterns for error classification, retry logic, graceful degradation, and API responses. The goal is to improve reliability, debuggability, and user experience when errors occur.

---

## Current State Analysis

### Existing Patterns

| Component | Current Approach | Issues |
|-----------|-----------------|--------|
| **Handlers** | RFC 7807 Problem Details format | ✅ Good: Standard format, but inconsistent error codes |
| **Repositories** | Direct GORM error returns | ⚠️ No wrapping, context loss |
| **Worker** | `log.Printf` for all errors | ❌ No retry, no classification |
| **Services** | `fmt.Errorf` with `%w` wrapping | ✅ Good: Error wrapping |
| **Middleware** | HTTP status + JSON response | ✅ Good: Consistent auth errors |

### Current Error Response Format

```go
// RFC 7807 Problem Details (used in handlers)
type ErrorResponse struct {
    Type    string `json:"type"`    // "about:blank" or error type URI
    Title   string `json:"title"`   // Human-readable title
    Status  int    `json:"status"`  // HTTP status code
    Detail  string `json:"detail"`  // Detailed error message
}
```

### Identified Problems

1. **No error classification**: All errors treated equally
2. **No retry logic**: Transient failures cause immediate failure
3. **No graceful degradation**: System fails hard on errors
4. **Inconsistent error codes**: Handlers use free-form strings
5. **Lost context**: Repository errors lose call stack
6. **No structured logging**: Plain `log.Printf` without context

---

## Decision

### 1. Error Classification

Define a standard error type hierarchy for AdHive:

```go
// internal/errors/errors.go

package errors

import (
    "errors"
    "fmt"
    "net/http"
)

// ErrorCategory represents the category of error
type ErrorCategory int

const (
    CategoryValidation ErrorCategory = iota
    CategoryNotFound
    CategoryUnauthorized
    CategoryForbidden
    CategoryConflict
    CategoryInternal
    CategoryExternal    // External service failures
    CategoryTransient   // Temporary failures, retry recommended
)

// ErrorCode represents a specific error type
type ErrorCode string

const (
    // Validation errors
    CodeInvalidInput       ErrorCode = "INVALID_INPUT"
    CodeInvalidEmail      ErrorCode = "INVALID_EMAIL"
    CodeInvalidPassword   ErrorCode = "INVALID_PASSWORD"
    CodeMissingField      ErrorCode = "MISSING_FIELD"
    
    // Authentication/Authorization
    CodeUnauthorized      ErrorCode = "UNAUTHORIZED"
    CodeSessionExpired    ErrorCode = "SESSION_EXPIRED"
    CodeForbidden         ErrorCode = "FORBIDDEN"
    
    // Resource errors
    CodeNotFound          ErrorCode = "NOT_FOUND"
    CodeEntryNotFound     ErrorCode = "ENTRY_NOT_FOUND"
    CodeTagNotFound       ErrorCode = "TAG_NOT_FOUND"
    
    // Conflict errors
    CodeDuplicateEntry    ErrorCode = "DUPLICATE_ENTRY"
    CodeDuplicateTag      ErrorCode = "DUPLICATE_TAG"
    
    // External service errors
    CodePlaywrightFailed  ErrorCode = "PLAYWRIGHT_FAILED"
    CodeArchiveFailed     ErrorCode = "ARCHIVE_FAILED"
    CodeThumbnailFailed   ErrorCode = "THUMBNAIL_FAILED"
    
    // Transient errors (retry recommended)
    CodeDatabaseBusy     ErrorCode = "DATABASE_BUSY"
    CodeRateLimited       ErrorCode = "RATE_LIMITED"
    CodeTimeout           ErrorCode = "TIMEOUT"
)

// AppError is the standard application error
type AppError struct {
    Code       ErrorCode
    Category   ErrorCategory
    Message    string
    Cause      error
    HTTPStatus int
    Retryable  bool
    Context    map[string]interface{} // Additional context
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
    return e.Cause
}

// Constructors for common errors
func NewValidationError(code ErrorCode, message string) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryValidation,
        Message:    message,
        HTTPStatus: http.StatusBadRequest,
        Retryable:  false,
    }
}

func NewNotFoundError(code ErrorCode, resource string) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryNotFound,
        Message:    fmt.Sprintf("%s not found", resource),
        HTTPStatus: http.StatusNotFound,
        Retryable:  false,
    }
}

func NewUnauthorizedError(code ErrorCode, message string) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryUnauthorized,
        Message:    message,
        HTTPStatus: http.StatusUnauthorized,
        Retryable:  false,
    }
}

func NewInternalError(code ErrorCode, message string, cause error) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryInternal,
        Message:    message,
        Cause:      cause,
        HTTPStatus: http.StatusInternalServerError,
        Retryable:  false,
    }
}

func NewTransientError(code ErrorCode, message string, cause error) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryTransient,
        Message:    message,
        Cause:      cause,
        HTTPStatus: http.StatusServiceUnavailable,
        Retryable:  true,
    }
}

func NewExternalError(code ErrorCode, message string, cause error) *AppError {
    return &AppError{
        Code:       code,
        Category:   CategoryExternal,
        Message:    message,
        Cause:      cause,
        HTTPStatus: http.StatusBadGateway,
        Retryable:  true,
    }
}

// Wrap wraps an error with context
func Wrap(cause error, code ErrorCode, message string) *AppError {
    return &AppError{
        Code:       code,
        Message:    message,
        Cause:      cause,
        HTTPStatus: http.StatusInternalServerError,
        Retryable:  false,
    }
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) (*AppError, bool) {
    var appErr *AppError
    if errors.As(err, &appErr) {
        return appErr, true
    }
    return nil, false
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
    if appErr, ok := IsAppError(err); ok {
        return appErr.Retryable
    }
    return false
}
```

---

### 2. Retry Logic

Define retry patterns for transient failures:

```go
// internal/retry/retry.go

package retry

import (
    "context"
    "errors"
    "math/rand"
    "time"
)

// Config holds retry configuration
type Config struct {
    MaxAttempts     int           // Maximum number of attempts
    InitialDelay    time.Duration // Initial delay between retries
    MaxDelay        time.Duration // Maximum delay cap
    Multiplier      float64       // Exponential backoff multiplier
    Jitter          float64       // Random jitter factor (0-1)
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
    return Config{
        MaxAttempts:  3,
        InitialDelay: 100 * time.Millisecond,
        MaxDelay:     5 * time.Second,
        Multiplier:   2.0,
        Jitter:       0.1,
    }
}

// ArchiveConfig returns config for archive operations (longer timeouts)
func ArchiveConfig() Config {
    return Config{
        MaxAttempts:  3,
        InitialDelay: 1 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
        Jitter:       0.2,
    }
}

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// IsRetryable checks if an error should trigger a retry
type IsRetryable func(error) bool

// Do executes a function with retry logic
func Do(ctx context.Context, config Config, isRetryable IsRetryable, fn RetryableFunc) error {
    var lastErr error
    delay := config.InitialDelay
    
    for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }
        
        lastErr = err
        
        // Check if we should retry
        if !isRetryable(err) {
            return err
        }
        
        // Check if this was the last attempt
        if attempt == config.MaxAttempts {
            break
        }
        
        // Calculate delay with jitter
        actualDelay := addJitter(delay, config.Jitter)
        
        // Wait or cancel
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(actualDelay):
        }
        
        // Increase delay for next attempt
        delay = time.Duration(float64(delay) * config.Multiplier)
        if delay > config.MaxDelay {
            delay = config.MaxDelay
        }
    }
    
    return lastErr
}

// DoWithResult executes a function that returns a result with retry logic
func DoWithResult[T any](ctx context.Context, config Config, isRetryable IsRetryable, fn func() (T, error)) (T, error) {
    var result T
    var lastErr error
    delay := config.InitialDelay
    
    for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
        res, err := fn()
        if err == nil {
            return res, nil
        }
        
        lastErr = err
        result = res
        
        if !isRetryable(err) {
            return result, err
        }
        
        if attempt == config.MaxAttempts {
            break
        }
        
        actualDelay := addJitter(delay, config.Jitter)
        
        select {
        case <-ctx.Done():
            return result, ctx.Err()
        case <-time.After(actualDelay):
        }
        
        delay = time.Duration(float64(delay) * config.Multiplier)
        if delay > config.MaxDelay {
            delay = config.MaxDelay
        }
    }
    
    return result, lastErr
}

func addJitter(delay time.Duration, jitter float64) time.Duration {
    if jitter <= 0 {
        return delay
    }
    variance := float64(delay) * jitter
    return delay + time.Duration(rand.Float64()*variance*2-variance)
}
```

---

### 3. Graceful Degradation Patterns

```go
// internal/degradation/degradation.go

package degradation

import (
    "context"
    "sync"
    "sync/atomic"
)

// Feature represents a feature that can be degraded
type Feature string

const (
    FeatureArchive     Feature = "archive"
    FeatureThumbnail   Feature = "thumbnail"
    FeaturePlaywright  Feature = "playwright"
    FeatureFTS5        Feature = "fts5"
)

// Mode represents the degradation level
type Mode int

const (
    ModeFull      Mode = iota // Full functionality
    ModeDegraded               // Reduced functionality
    ModeDisabled               // Feature disabled
)

// Manager handles graceful degradation
type Manager struct {
    mu       sync.RWMutex
    features map[Feature]Mode
}

// NewManager creates a new degradation manager
func NewManager() *Manager {
    return &Manager{
        features: make(map[Feature]Mode),
    }
}

// SetMode sets the degradation mode for a feature
func (m *Manager) SetMode(feature Feature, mode Mode) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.features[feature] = mode
}

// GetMode gets the current mode for a feature
func (m *Manager) GetMode(feature Feature) Mode {
    m.mu.RLock()
    defer m.mu.RUnlock()
    if mode, ok := m.features[feature]; ok {
        return mode
    }
    return ModeFull
}

// IsAvailable checks if a feature is available
func (m *Manager) IsAvailable(feature Feature) bool {
    return m.GetMode(feature) != ModeDisabled
}

// CanUse checks if a feature can be used (not disabled)
func (m *Manager) CanUse(feature Feature) bool {
    mode := m.GetMode(feature)
    return mode == ModeFull || mode == ModeDegraded
}

// DegradedFallback executes a primary function with fallback
func (m *Manager) DegradedFallback(ctx context.Context, feature Feature, primary, fallback func() error) error {
    mode := m.GetMode(feature)
    
    switch mode {
    case ModeDisabled:
        return fallback()
    case ModeDegraded:
        // Try primary, fallback on failure
        if err := primary(); err != nil {
            return fallback()
        }
        return nil
    case ModeFull:
        return primary()
    default:
        return primary()
    }
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
    maxFailures   int32
    failures      int32
    state         int32 // 0=closed, 1=open, 2=half-open
    resetAfter    time.Duration
    lastFailure   atomic.Int64
    mu            sync.Mutex
}

func NewCircuitBreaker(maxFailures int, resetAfter time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        maxFailures: int32(maxFailures),
        resetAfter:  resetAfter,
    }
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    // Check if circuit is open
    if atomic.LoadInt32(&cb.state) == 1 {
        // Check if we should try half-open
        lastFailure := time.Unix(0, cb.lastFailure.Load())
        if time.Since(lastFailure) > cb.resetAfter {
            cb.mu.Lock()
            if atomic.LoadInt32(&cb.state) == 1 {
                atomic.StoreInt32(&cb.state, 2) // half-open
            }
            cb.mu.Unlock()
        } else {
            return ErrCircuitOpen
        }
    }
    
    err := fn()
    
    if err != nil {
        cb.recordFailure()
        return err
    }
    
    cb.recordSuccess()
    return nil
}

func (cb *CircuitBreaker) recordFailure() {
    failures := atomic.AddInt32(&cb.failures, 1)
    cb.lastFailure.Store(time.Now().UnixNano())
    if failures >= cb.maxFailures {
        atomic.StoreInt32(&cb.state, 1) // open
    }
}

func (cb *CircuitBreaker) recordSuccess() {
    atomic.StoreInt32(&cb.failures, 0)
    atomic.StoreInt32(&cb.state, 0) // closed
}

var ErrCircuitOpen = errors.New("circuit breaker is open")
```

---

### 4. Standardized API Error Responses

```go
// internal/handler/errors.go

package handler

import (
    "errors"
    "net/http"
    
    "github.com/carev01/adhive/internal/errors"
    "github.com/gin-gonic/gin"
)

// ErrorResponse follows RFC 7807 Problem Details format
type ErrorResponse struct {
    Type     string `json:"type"`           // URI reference for error type
    Title    string `json:"title"`          // Human-readable title
    Status   int    `json:"status"`         // HTTP status code
    Detail   string `json:"detail"`         // Human-readable details
    Code     string `json:"code,omitempty"` // Application-specific error code
    TraceID  string `json:"trace_id,omitempty"`
}

// ErrorCodeRegistry maps error codes to HTTP status
var ErrorCodeRegistry = map[errors.ErrorCode]int{
    // Validation errors
    errors.CodeInvalidInput:     http.StatusBadRequest,
    errors.CodeInvalidEmail:     http.StatusBadRequest,
    errors.CodeInvalidPassword:  http.StatusBadRequest,
    errors.CodeMissingField:     http.StatusBadRequest,
    
    // Auth errors
    errors.CodeUnauthorized:     http.StatusUnauthorized,
    errors.CodeSessionExpired:   http.StatusUnauthorized,
    errors.CodeForbidden:        http.StatusForbidden,
    
    // Resource errors
    errors.CodeNotFound:         http.StatusNotFound,
    errors.CodeEntryNotFound:    http.StatusNotFound,
    errors.CodeTagNotFound:      http.StatusNotFound,
    
    // Conflict errors
    errors.CodeDuplicateEntry:    http.StatusConflict,
    errors.CodeDuplicateTag:     http.StatusConflict,
    
    // External errors
    errors.CodePlaywrightFailed: http.StatusBadGateway,
    errors.CodeArchiveFailed:    http.StatusBadGateway,
    errors.CodeThumbnailFailed:  http.StatusBadGateway,
    
    // Transient errors
    errors.CodeDatabaseBusy:     http.StatusServiceUnavailable,
    errors.CodeRateLimited:      http.StatusTooManyRequests,
    errors.CodeTimeout:          http.StatusGatewayTimeout,
}

// SendError sends a standardized error response
func SendError(c *gin.Context, err error) {
    appErr, ok := errors.IsAppError(err)
    
    if !ok {
        // Wrap unknown errors
        appErr = errors.NewInternalError("INTERNAL_ERROR", "An unexpected error occurred", err)
    }
    
    status := appErr.HTTPStatus
    if status == 0 {
        status = http.StatusInternalServerError
    }
    
    // Get trace ID from context if available
    traceID, _ := c.Get("trace_id")
    
    c.JSON(status, ErrorResponse{
        Type:    string(appErr.Code),
        Title:   appErr.Code.Title(),
        Status:  status,
        Detail:  appErr.Message,
        Code:    string(appErr.Code),
        TraceID: traceIDStr(traceID),
    })
}

// SendErrorWithDetail sends an error with additional detail
func SendErrorWithDetail(c *gin.Context, err error, detail string) {
    appErr, ok := errors.IsAppError(err)
    if !ok {
        appErr = errors.NewInternalError("INTERNAL_ERROR", detail, err)
    } else {
        appErr.Message = detail
    }
    SendError(c, appErr)
}

func traceIDStr(v interface{}) string {
    if v == nil {
        return ""
    }
    if s, ok := v.(string); ok {
        return s
    }
    return ""
}
```

---

### 5. Logging & Observability

```go
// internal/logging/logging.go

package logging

import (
    "context"
    "log/slog"
    "os"
)

// ContextKey for request-scoped values
type ContextKey string

const (
    ContextKeyTraceID   ContextKey = "trace_id"
    ContextKeyUserID     ContextKey = "user_id"
    ContextKeyEntryID    ContextKey = "entry_id"
)

// Logger wraps slog with structured logging
type Logger struct {
    *slog.Logger
}

var defaultLogger *Logger

func init() {
    defaultLogger = &Logger{
        Logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
            Level: slog.LevelInfo,
        })),
    }
}

// Default returns the default logger
func Default() *Logger {
    return defaultLogger
}

// WithContext returns a logger with context values
func (l *Logger) WithContext(ctx context.Context) *Logger {
    attrs := []slog.Attr{}
    
    if traceID := ctx.Value(ContextKeyTraceID); traceID != nil {
        attrs = append(attrs, slog.String("trace_id", traceID.(string)))
    }
    if userID := ctx.Value(ContextKeyUserID); userID != nil {
        attrs = append(attrs, slog.String("user_id", userID.(string)))
    }
    
    return &Logger{
        Logger: l.WithAttrs(attrs),
    }
}

// Error logs an error with context
func (l *Logger) Error(err error, msg string, args ...any) {
    attrs := []slog.Attr{slog.Any("error", err)}
    l.Logger.Error(msg, attrs...)
}

// LogError logs with structured fields
func (l *Logger) LogError(ctx context.Context, err error, message string, fields ...slog.Attr) {
    l.WithContext(ctx).
        WithAttrs(fields...).
        Error(err, message)
}

// LogOperation logs start/end of operations
func (l *Logger) LogOperation(ctx context.Context, operation string, fn func() error) error {
    l.WithContext(ctx).Info("starting operation", "operation", operation)
    
    start := time.Now()
    err := fn()
    duration := time.Since(start)
    
    if err != nil {
        l.WithContext(ctx).Error(err, "operation failed",
            "operation", operation,
            "duration_ms", duration.Milliseconds(),
        )
        return err
    }
    
    l.WithContext(ctx).Info("operation completed",
        "operation", operation,
        "duration_ms", duration.Milliseconds(),
    )
    return nil
}
```

---

## Implementation Plan

### Phase 1: Error Types Package (1 day)

1. Create `internal/errors/errors.go` with AppError type
2. Define all error codes
3. Add constructors for each error category
4. Add unit tests

### Phase 2: Retry Package (1 day)

1. Create `internal/retry/retry.go`
2. Implement exponential backoff
3. Add circuit breaker for external services
4. Add unit tests

### Phase 3: Handler Integration (1 day)

1. Update `internal/handler/errors.go` with new error handling
2. Add `SendError` function
3. Create error code registry
4. Update existing handlers to use new error types

### Phase 4: Worker Integration (1 day)

1. Update `internal/worker/archive.go` to use retry logic
2. Implement graceful degradation for Playwright failures
3. Add circuit breaker for external operations
4. Replace `log.Printf` with structured logging

### Phase 5: Logging Integration (1 day)

1. Create `internal/logging/logging.go`
2. Add request tracing middleware
3. Update all services to use structured logging
4. Add metrics emission

---

## Code Examples: Before & After

### Before (Current)

```go
// Repository: Direct error return
func (r *EntryRepository) GetByID(ctx context.Context, id string) (*model.CatalogEntry, error) {
    var entry model.CatalogEntry
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
    if err != nil {
        return nil, err  // No wrapping, no context
    }
    return &entry, nil
}

// Handler: Inline error response
func (h *EntryHandler) GetByID(c *gin.Context) {
    entry, err := h.entryRepo.GetByID(ctx, id)
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            c.JSON(http.StatusNotFound, ErrorResponse{
                Type:   "about:blank",
                Title:  "Not Found",
                Status: http.StatusNotFound,
                Detail: "entry not found",
            })
            return
        }
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Type:   "about:blank",
            Title:  "Internal Server Error",
            Status: http.StatusInternalServerError,
            Detail: "failed to get entry",
        })
        return
    }
    // ...
}

// Worker: Log and continue
if err != nil {
    log.Printf("Error finding entry %s: %v", entryID, err)
    return
}
```

### After (Proposed)

```go
// Repository: Wrapped error with context
func (r *EntryRepository) GetByID(ctx context.Context, id string) (*model.CatalogEntry, error) {
    var entry model.CatalogEntry
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, errors.NewNotFoundError(errors.CodeEntryNotFound, "entry").
                WithContext("entry_id", id)
        }
        return nil, errors.NewInternalError(errors.CodeInternalError, "failed to get entry", err).
            WithContext("entry_id", id)
    }
    return &entry, nil
}

// Handler: Centralized error handling
func (h *EntryHandler) GetByID(c *gin.Context) {
    entry, err := h.entryRepo.GetByID(ctx, id)
    if err != nil {
        SendError(c, err)
        return
    }
    // ...
}

// Worker: Structured logging with retry
func (w *ArchiveWorker) processEntry(ctx context.Context, entryID string) error {
    return retry.Do(ctx, retry.ArchiveConfig(), errors.IsRetryable, func() error {
        logger := logging.Default().WithContext(ctx).
            With(slog.String("entry_id", entryID))
        
        logger.Info("starting archive")
        
        entry, err := w.entryRepo.GetByID(ctx, entryID)
        if err != nil {
            logger.Error(err, "failed to get entry")
            return err  // Will be classified by IsRetryable
        }
        
        // Use degradation manager for Playwright
        return w.degradation.DegradedFallback(ctx, degradation.FeaturePlaywright,
            func() error {
                return w.captureWithPlaywright(ctx, entry)
            },
            func() error {
                return w.captureWithHTTP(ctx, entry)  // Fallback
            },
        )
    })
}
```

---

## Migration Strategy

### Gradual Adoption

1. **Phase 1**: Add `internal/errors` package without changing existing code
2. **Phase 2**: Update handlers to use `SendError`
3. **Phase 3**: Update repositories to wrap errors
4. **Phase 4**: Update worker to use retry + structured logging
5. **Phase 5**: Remove old error patterns

### Backward Compatibility

- Keep existing `ErrorResponse` struct for API compatibility
- New `Code` field is optional
- `type` field can be `"about:blank"` or error code URI

---

## Testing Strategy

### Unit Tests

```go
func TestAppError(t *testing.T) {
    err := errors.NewNotFoundError(errors.CodeEntryNotFound, "entry")
    
    assert.Equal(t, errors.CodeEntryNotFound, err.Code)
    assert.Equal(t, errors.CategoryNotFound, err.Category)
    assert.Equal(t, http.StatusNotFound, err.HTTPStatus)
    assert.False(t, err.Retryable)
}

func TestRetryLogic(t *testing.T) {
    attempts := 0
    
    err := retry.Do(context.Background(), retry.DefaultConfig(), 
        func(error) bool { return true },  // Always retryable
        func() error {
            attempts++
            if attempts < 3 {
                return errors.NewTransientError("TEMP", "temporary error", nil)
            }
            return nil
        },
    )
    
    assert.NoError(t, err)
    assert.Equal(t, 3, attempts)
}

func TestCircuitBreaker(t *testing.T) {
    cb := degradation.NewCircuitBreaker(2, time.Second)
    
    // First failure
    err := cb.Call(func() error { return errors.New("fail") })
    assert.Error(t, err)
    
    // Second failure - should open circuit
    err = cb.Call(func() error { return errors.New("fail") })
    assert.Error(t, err)
    
    // Circuit should be open
    err = cb.Call(func() error { return nil })
    assert.Equal(t, degradation.ErrCircuitOpen, err)
}
```

---

## Metrics & Monitoring

### Recommended Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `errors_total` | Counter | Total errors by code |
| `errors_by_category` | Counter | Errors grouped by category |
| `retries_total` | Counter | Retry attempts by operation |
| `circuit_breaker_state` | Gauge | Circuit breaker states |
| `degradation_mode` | Gauge | Feature degradation modes |
| `request_duration_ms` | Histogram | Request duration by endpoint |

### Alerting Rules

1. **Error Rate Spike**: `rate(errors_total) > 10/min`
2. **Circuit Breaker Open**: `circuit_breaker_state == 1` for > 5min
3. **High Retry Rate**: `rate(retries_total) > 100/min`

---

## Appendix A: Error Code Registry

| Code | Category | HTTP Status | Retryable | Description |
|------|----------|-------------|-----------|-------------|
| `INVALID_INPUT` | Validation | 400 | No | Invalid request data |
| `INVALID_EMAIL` | Validation | 400 | No | Invalid email format |
| `INVALID_PASSWORD` | Validation | 400 | No | Password doesn't meet requirements |
| `MISSING_FIELD` | Validation | 400 | No | Required field missing |
| `UNAUTHORIZED` | Auth | 401 | No | Not authenticated |
| `SESSION_EXPIRED` | Auth | 401 | No | Session has expired |
| `FORBIDDEN` | Auth | 403 | No | Not authorized |
| `NOT_FOUND` | Resource | 404 | No | Resource not found |
| `ENTRY_NOT_FOUND` | Resource | 404 | No | Entry not found |
| `TAG_NOT_FOUND` | Resource | 404 | No | Tag not found |
| `DUPLICATE_ENTRY` | Conflict | 409 | No | Entry already exists |
| `DUPLICATE_TAG` | Conflict | 409 | No | Tag already exists |
| `PLAYWRIGHT_FAILED` | External | 502 | Yes | Playwright capture failed |
| `ARCHIVE_FAILED` | External | 502 | Yes | Archive creation failed |
| `THUMBNAIL_FAILED` | External | 502 | Yes | Thumbnail generation failed |
| `DATABASE_BUSY` | Transient | 503 | Yes | Database locked |
| `RATE_LIMITED` | Transient | 429 | Yes | Rate limit exceeded |
| `TIMEOUT` | Transient | 504 | Yes | Operation timed out |
| `INTERNAL_ERROR` | Internal | 500 | No | Unexpected error |

---

## Appendix B: Integration Checklist

### Handlers

- [ ] Import `internal/errors`
- [ ] Replace inline `ErrorResponse` with `SendError(c, err)`
- [ ] Use `errors.IsAppError()` to check error types
- [ ] Add context values for tracing

### Repositories

- [ ] Wrap GORM errors with `errors.NewNotFoundError()`
- [ ] Wrap internal errors with `errors.NewInternalError()`
- [ ] Add context to errors with `WithContext()`

### Services

- [ ] Use `errors.NewExternalError()` for external failures
- [ ] Use `errors.NewTransientError()` for retryable errors
- [ ] Use `retry.Do()` for retryable operations

### Worker

- [ ] Replace `log.Printf` with structured logging
- [ ] Add retry logic for transient failures
- [ ] Implement graceful degradation for Playwright
- [ ] Add circuit breaker for external services

---

*End of ADR-004*