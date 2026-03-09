// Package errors provides error types and utilities for AdHive.
package errors

import (
	"errors"
	"fmt"
)

// ErrorCategory represents the category of an error.
type ErrorCategory string

const (
	// CategoryValidation represents input validation errors.
	CategoryValidation ErrorCategory = "validation"
	// CategoryNotFound represents resource not found errors.
	CategoryNotFound ErrorCategory = "not_found"
	// CategoryUnauthorized represents authentication errors.
	CategoryUnauthorized ErrorCategory = "unauthorized"
	// CategoryForbidden represents authorization errors.
	CategoryForbidden ErrorCategory = "forbidden"
	// CategoryConflict represents conflict errors (e.g., duplicate).
	CategoryConflict ErrorCategory = "conflict"
	// CategoryInternal represents internal server errors.
	CategoryInternal ErrorCategory = "internal"
	// CategoryExternal represents errors from external services.
	CategoryExternal ErrorCategory = "external"
	// CategoryTransient represents transient errors that may succeed on retry.
	CategoryTransient ErrorCategory = "transient"
)

// ErrorCode represents a specific error code.
type ErrorCode string

// Validation error codes.
const (
	CodeInvalidInput    ErrorCode = "INVALID_INPUT"
	CodeInvalidEmail    ErrorCode = "INVALID_EMAIL"
	CodeInvalidPassword ErrorCode = "INVALID_PASSWORD"
	CodeMissingField    ErrorCode = "MISSING_FIELD"
	CodeInvalidFormat   ErrorCode = "INVALID_FORMAT"
)

// Authorization error codes.
const (
	CodeUnauthorized    ErrorCode = "UNAUTHORIZED"
	CodeSessionExpired  ErrorCode = "SESSION_EXPIRED"
	CodeForbidden       ErrorCode = "FORBIDDEN"
	CodeInvalidToken    ErrorCode = "INVALID_TOKEN"
)

// Resource error codes.
const (
	CodeNotFound      ErrorCode = "NOT_FOUND"
	CodeEntryNotFound ErrorCode = "ENTRY_NOT_FOUND"
	CodeTagNotFound   ErrorCode = "TAG_NOT_FOUND"
	CodeUserNotFound  ErrorCode = "USER_NOT_FOUND"
)

// Conflict error codes.
const (
	CodeDuplicateEntry  ErrorCode = "DUPLICATE_ENTRY"
	CodeDuplicateTag    ErrorCode = "DUPLICATE_TAG"
	CodeDuplicateUser   ErrorCode = "DUPLICATE_USER"
)

// External service error codes.
const (
	CodePlaywrightFailed  ErrorCode = "PLAYWRIGHT_FAILED"
	CodeArchiveFailed    ErrorCode = "ARCHIVE_FAILED"
	CodeThumbnailFailed  ErrorCode = "THUMBNAIL_FAILED"
	CodeExternalService  ErrorCode = "EXTERNAL_SERVICE_ERROR"
)

// Transient error codes (retryable).
const (
	CodeDatabaseBusy       ErrorCode = "DATABASE_BUSY"
	CodeRateLimited       ErrorCode = "RATE_LIMITED"
	CodeTimeout           ErrorCode = "TIMEOUT"
	CodeTemporaryFailure  ErrorCode = "TEMPORARY_FAILURE"
)

// Internal error codes.
const (
	CodeInternal ErrorCode = "INTERNAL_ERROR"
)

// AppError represents an application error with structured information.
type AppError struct {
	Code       ErrorCode             `json:"code"`
	Category   ErrorCategory         `json:"category"`
	Message    string                `json:"message"`
	Cause      error                 `json:"-"`
	HTTPStatus int                   `json:"-"`
	Retryable  bool                  `json:"retryable"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause of the error.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// IsAppError checks if an error is an AppError and returns it.
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	appErr, ok := IsAppError(err)
	if ok {
		return appErr.Retryable
	}
	// Default to false for unknown errors
	return false
}

// NewValidationError creates a new validation error.
func NewValidationError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryValidation,
		Message:    message,
		HTTPStatus: 400,
		Retryable:  false,
	}
}

// NewNotFoundError creates a new not found error.
func NewNotFoundError(code ErrorCode, resource string) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryNotFound,
		Message:    fmt.Sprintf("%s not found", resource),
		HTTPStatus: 404,
		Retryable:  false,
	}
}

// NewUnauthorizedError creates a new unauthorized error.
func NewUnauthorizedError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryUnauthorized,
		Message:    message,
		HTTPStatus: 401,
		Retryable:  false,
	}
}

// NewForbiddenError creates a new forbidden error.
func NewForbiddenError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryForbidden,
		Message:    message,
		HTTPStatus: 403,
		Retryable:  false,
	}
}

// NewConflictError creates a new conflict error.
func NewConflictError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryConflict,
		Message:    message,
		HTTPStatus: 409,
		Retryable:  false,
	}
}

// NewInternalError creates a new internal error.
func NewInternalError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryInternal,
		Message:    message,
		Cause:      cause,
		HTTPStatus: 500,
		Retryable:  false,
	}
}

// NewTransientError creates a new transient error (retryable).
func NewTransientError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryTransient,
		Message:    message,
		Cause:      cause,
		HTTPStatus: 503,
		Retryable:  true,
	}
}

// NewExternalError creates a new external service error.
func NewExternalError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Category:   CategoryExternal,
		Message:    message,
		Cause:      cause,
		HTTPStatus: 502,
		Retryable:  true,
	}
}

// WithContext adds context to an AppError.
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}
