package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name: "simple error without cause",
			err: &AppError{
				Code:    CodeInvalidInput,
				Message: "invalid input provided",
			},
			expected: "INVALID_INPUT: invalid input provided",
		},
		{
			name: "error with cause",
			err: &AppError{
				Code:    CodeInternal,
				Message: "database error",
				Cause:   errors.New("connection refused"),
			},
			expected: "INTERNAL_ERROR: database error (caused by: connection refused)",
		},
		{
			name: "error with empty message",
			err: &AppError{
				Code:    CodeNotFound,
				Message: "",
			},
			expected: "NOT_FOUND: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	err := &AppError{
		Code:    CodeInternal,
		Message: "internal error",
		Cause:   cause,
	}

	got := err.Unwrap()
	if got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestAppError_Unwrap_NilCause(t *testing.T) {
	err := &AppError{
		Code:    CodeNotFound,
		Message: "not found",
		Cause:   nil,
	}

	got := err.Unwrap()
	if got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestIsAppError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantFound bool
		wantCode  ErrorCode
	}{
		{
			name:      "is AppError",
			err:       NewValidationError(CodeInvalidInput, "invalid"),
			wantFound: true,
			wantCode:  CodeInvalidInput,
		},
		{
			name:      "is AppError not found",
			err:       NewNotFoundError(CodeEntryNotFound, "entry"),
			wantFound: true,
			wantCode:  CodeEntryNotFound,
		},
		{
			name:      "is AppError with nested",
			err:       NewInternalError(CodeInternal, "error", errors.New("cause")),
			wantFound: true,
			wantCode:  CodeInternal,
		},
		{
			name:      "is not AppError - standard error",
			err:       errors.New("standard error"),
			wantFound: false,
		},
		{
			name:      "is not AppError - wrapped error",
			err:       fmt.Errorf("wrapped: %w", errors.New("inner")),
			wantFound: false,
		},
		{
			name:      "is not AppError - nil",
			err:       nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := IsAppError(tt.err)
			if found != tt.wantFound {
				t.Errorf("IsAppError() found = %v, want %v", found, tt.wantFound)
			}
			if found && tt.wantFound && got.Code != tt.wantCode {
				t.Errorf("IsAppError() code = %v, want %v", got.Code, tt.wantCode)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantRetry bool
	}{
		{
			name:      "validation error is not retryable",
			err:       NewValidationError(CodeInvalidInput, "invalid"),
			wantRetry: false,
		},
		{
			name:      "not found error is not retryable",
			err:       NewNotFoundError(CodeEntryNotFound, "entry"),
			wantRetry: false,
		},
		{
			name:      "transient error is retryable",
			err:       NewTransientError(CodeDatabaseBusy, "database busy", nil),
			wantRetry: true,
		},
		{
			name:      "external error is retryable",
			err:       NewExternalError(CodePlaywrightFailed, "playwright failed", nil),
			wantRetry: true,
		},
		{
			name:      "standard error is not retryable",
			err:       errors.New("standard error"),
			wantRetry: false,
		},
		{
			name:      "nil error returns false",
			err:       nil,
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError(CodeInvalidInput, "email is invalid")

	if err.Code != CodeInvalidInput {
		t.Errorf("Code = %v, want %v", err.Code, CodeInvalidInput)
	}
	if err.Category != CategoryValidation {
		t.Errorf("Category = %v, want %v", err.Category, CategoryValidation)
	}
	if err.Message != "email is invalid" {
		t.Errorf("Message = %v, want %v", err.Message, "email is invalid")
	}
	if err.HTTPStatus != 400 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 400)
	}
	if err.Retryable != false {
		t.Errorf("Retryable = %v, want false", err.Retryable)
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError(CodeEntryNotFound, "bookmark")

	if err.Code != CodeEntryNotFound {
		t.Errorf("Code = %v, want %v", err.Code, CodeEntryNotFound)
	}
	if err.Category != CategoryNotFound {
		t.Errorf("Category = %v, want %v", err.Category, CategoryNotFound)
	}
	if err.Message != "bookmark not found" {
		t.Errorf("Message = %v, want %v", err.Message, "bookmark not found")
	}
	if err.HTTPStatus != 404 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 404)
	}
}

func TestNewUnauthorizedError(t *testing.T) {
	err := NewUnauthorizedError(CodeSessionExpired, "session expired")

	if err.Code != CodeSessionExpired {
		t.Errorf("Code = %v, want %v", err.Code, CodeSessionExpired)
	}
	if err.Category != CategoryUnauthorized {
		t.Errorf("Category = %v, want %v", err.Category, CategoryUnauthorized)
	}
	if err.HTTPStatus != 401 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 401)
	}
}

func TestNewForbiddenError(t *testing.T) {
	err := NewForbiddenError(CodeForbidden, "access denied")

	if err.Code != CodeForbidden {
		t.Errorf("Code = %v, want %v", err.Code, CodeForbidden)
	}
	if err.Category != CategoryForbidden {
		t.Errorf("Category = %v, want %v", err.Category, CategoryForbidden)
	}
	if err.HTTPStatus != 403 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 403)
	}
}

func TestNewConflictError(t *testing.T) {
	err := NewConflictError(CodeDuplicateEntry, "entry already exists")

	if err.Code != CodeDuplicateEntry {
		t.Errorf("Code = %v, want %v", err.Code, CodeDuplicateEntry)
	}
	if err.Category != CategoryConflict {
		t.Errorf("Category = %v, want %v", err.Category, CategoryConflict)
	}
	if err.HTTPStatus != 409 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 409)
	}
}

func TestNewInternalError(t *testing.T) {
	cause := errors.New("database connection failed")
	err := NewInternalError(CodeInternal, "internal server error", cause)

	if err.Code != CodeInternal {
		t.Errorf("Code = %v, want %v", err.Code, CodeInternal)
	}
	if err.Category != CategoryInternal {
		t.Errorf("Category = %v, want %v", err.Category, CategoryInternal)
	}
	if err.HTTPStatus != 500 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 500)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewTransientError(t *testing.T) {
	err := NewTransientError(CodeDatabaseBusy, "database is busy", nil)

	if err.Code != CodeDatabaseBusy {
		t.Errorf("Code = %v, want %v", err.Code, CodeDatabaseBusy)
	}
	if err.Category != CategoryTransient {
		t.Errorf("Category = %v, want %v", err.Category, CategoryTransient)
	}
	if err.HTTPStatus != 503 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 503)
	}
	if !err.Retryable {
		t.Error("Retryable should be true for transient errors")
	}
}

func TestNewExternalError(t *testing.T) {
	err := NewExternalError(CodePlaywrightFailed, "playwright failed", errors.New("timeout"))

	if err.Code != CodePlaywrightFailed {
		t.Errorf("Code = %v, want %v", err.Code, CodePlaywrightFailed)
	}
	if err.Category != CategoryExternal {
		t.Errorf("Category = %v, want %v", err.Category, CategoryExternal)
	}
	if err.HTTPStatus != 502 {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, 502)
	}
	if !err.Retryable {
		t.Error("Retryable should be true for external errors")
	}
}

func TestAppError_WithContext(t *testing.T) {
	err := NewValidationError(CodeInvalidInput, "invalid input")

	// Test adding context
	err = err.WithContext("field", "email")
	err = err.WithContext("value", "not-an-email")

	if err.Context == nil {
		t.Fatal("Context should not be nil after WithContext")
	}

	if err.Context["field"] != "email" {
		t.Errorf("Context[field] = %v, want 'email'", err.Context["field"])
	}
	if err.Context["value"] != "not-an-email" {
		t.Errorf("Context[value] = %v, want 'not-an-email'", err.Context["value"])
	}

	// Test chaining
	err2 := NewNotFoundError(CodeEntryNotFound, "entry").
		WithContext("entry_id", "123")

	if err2.Context["entry_id"] != "123" {
		t.Errorf("Context[entry_id] = %v, want '123'", err2.Context["entry_id"])
	}
}

func TestAppError_WithContext_Empty(t *testing.T) {
	err := &AppError{}

	// Adding context to error with nil Context map
	err = err.WithContext("key", "value")

	if err.Context == nil {
		t.Error("Context should be initialized if nil")
	}
	if err.Context["key"] != "value" {
		t.Errorf("Context[key] = %v, want 'value'", err.Context["key"])
	}
}
