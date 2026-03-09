package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestErrorResponse(t *testing.T) {
	// Test ErrorResponse struct can be created
	resp := ErrorResponse{
		Type:    "about:blank",
		Title:   "validation",
		Status:  400,
		Detail:  "invalid input",
		Code:    "INVALID_INPUT",
		Context: map[string]interface{}{"field": "email"},
	}

	if resp.Type != "about:blank" {
		t.Errorf("Type = %v, want about:blank", resp.Type)
	}
	if resp.Title != "validation" {
		t.Errorf("Title = %v, want validation", resp.Title)
	}
	if resp.Status != 400 {
		t.Errorf("Status = %v, want 400", resp.Status)
	}
	if resp.Code != "INVALID_INPUT" {
		t.Errorf("Code = %v, want INVALID_INPUT", resp.Code)
	}
	if resp.Context["field"] != "email" {
		t.Errorf("Context[field] = %v, want email", resp.Context["field"])
	}
}

func TestErrorCodeRegistry(t *testing.T) {
	tests := []struct {
		code      apperrors.ErrorCode
		wantStatus int
	}{
		{apperrors.CodeInvalidInput, http.StatusBadRequest},
		{apperrors.CodeInvalidEmail, http.StatusBadRequest},
		{apperrors.CodeUnauthorized, http.StatusUnauthorized},
		{apperrors.CodeForbidden, http.StatusForbidden},
		{apperrors.CodeNotFound, http.StatusNotFound},
		{apperrors.CodeEntryNotFound, http.StatusNotFound},
		{apperrors.CodeDuplicateEntry, http.StatusConflict},
		{apperrors.CodePlaywrightFailed, http.StatusBadGateway},
		{apperrors.CodeDatabaseBusy, http.StatusServiceUnavailable},
		{apperrors.CodeRateLimited, http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			status, found := ErrorCodeRegistry[tt.code]
			if !found {
				t.Errorf("Code %v not found in registry", tt.code)
			}
			if status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

func TestSendError_ValidationError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewValidationError(apperrors.CodeInvalidInput, "email is required")
	SendError(c, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	// Verify response body contains expected fields
	body := w.Body.String()
	if !contains(body, "validation") {
		t.Error("Response should contain title 'validation'")
	}
	if !contains(body, "INVALID_INPUT") {
		t.Error("Response should contain code 'INVALID_INPUT'")
	}
	if !contains(body, "email is required") {
		t.Error("Response should contain detail 'email is required'")
	}
}

func TestSendError_NotFoundError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry")
	SendError(c, err)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusNotFound)
	}

	body := w.Body.String()
	if !contains(body, "not_found") {
		t.Error("Response should contain title 'not_found'")
	}
}

func TestSendError_UnauthorizedError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "not authenticated")
	SendError(c, err)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusUnauthorized)
	}
}

func TestSendError_ForbiddenError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewForbiddenError(apperrors.CodeForbidden, "access denied")
	SendError(c, err)

	if w.Code != http.StatusForbidden {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusForbidden)
	}
}

func TestSendError_ConflictError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewConflictError(apperrors.CodeDuplicateEntry, "entry already exists")
	SendError(c, err)

	if w.Code != http.StatusConflict {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusConflict)
	}
}

func TestSendError_InternalError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	cause := errors.New("database error")
	err := apperrors.NewInternalError(apperrors.CodeInternal, "failed to save", cause)
	SendError(c, err)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusInternalServerError)
	}

	body := w.Body.String()
	if !contains(body, "internal") {
		t.Error("Response should contain title 'internal'")
	}
}

func TestSendError_TransientError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewTransientError(apperrors.CodeDatabaseBusy, "database busy", nil)
	SendError(c, err)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusServiceUnavailable)
	}
}

func TestSendError_ExternalError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewExternalError(apperrors.CodePlaywrightFailed, "playwright failed", errors.New("timeout"))
	SendError(c, err)

	if w.Code != http.StatusBadGateway {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusBadGateway)
	}
}

func TestSendError_WithContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := apperrors.NewValidationError(apperrors.CodeInvalidInput, "invalid field").
		WithContext("field", "email").
		WithContext("value", "not-an-email")
	SendError(c, err)

	body := w.Body.String()
	if !contains(body, "email") {
		t.Error("Response should contain context field 'email'")
	}
}

func TestSendError_NilError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Should not panic
	SendError(c, nil)

	if w.Code != http.StatusOK {
		// Nil error should not set any status (default 200)
		// But the function returns early, so no response is sent
	}
}

func TestSendError_NonAppError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Standard error (not AppError)
	err := errors.New("standard error")
	SendError(c, err)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusInternalServerError)
	}

	body := w.Body.String()
	if !contains(body, "unexpected error") {
		t.Error("Response should contain generic error message")
	}
}

func TestSendErrorWithStatus(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	SendErrorWithStatus(c, http.StatusTooManyRequests, "Rate Limited", "too many requests")

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusTooManyRequests)
	}

	body := w.Body.String()
	if !contains(body, "Rate Limited") {
		t.Error("Response should contain title 'Rate Limited'")
	}
	if !contains(body, "too many requests") {
		t.Error("Response should contain detail 'too many requests'")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
