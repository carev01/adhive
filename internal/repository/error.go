// Package repository provides database operations for AdHive models.
package repository

import (
	"errors"

	apperrors "github.com/carev01/adhive/internal/errors"
	"gorm.io/gorm"
)

// WrapDBError wraps a GORM error with an appropriate AppError.
// It classifies errors into NotFound, Conflict, Transient, or Internal categories.
func WrapDBError(err error, resourceType string, resourceID string) error {
	if err == nil {
		return nil
	}

	if _, ok := apperrors.IsAppError(err); ok {
		// Already an AppError, just add context
		if resourceID != "" {
			return apperrors.NewInternalError(apperrors.CodeInternal, "Database error", err).
				WithContext("resource_type", resourceType).
				WithContext("resource_id", resourceID)
		}
		return err
	}

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperrors.NewNotFoundError(apperrors.CodeNotFound, resourceType).
			WithContext("resource_id", resourceID)

	case errors.Is(err, gorm.ErrDuplicatedKey):
		return apperrors.NewConflictError(apperrors.CodeDuplicateEntry, "Duplicate entry for "+resourceType).
			WithContext("resource_id", resourceID)

	case isTransientError(err):
		return apperrors.NewTransientError(apperrors.CodeDatabaseBusy, "Database temporarily unavailable", err).
			WithContext("resource_type", resourceType).
			WithContext("resource_id", resourceID)

	default:
		return apperrors.NewInternalError(apperrors.CodeInternal, "Database error: "+err.Error(), err).
			WithContext("resource_type", resourceType).
			WithContext("resource_id", resourceID)
	}
}

// isTransientError checks if a GORM error is transient and worth retrying.
func isTransientError(err error) bool {
	errStr := err.Error()
	// Check for common transient SQLite errors
	transientMarkers := []string{
		"database is locked",
		"database busy",
		"too many clients",
		"connection refused",
		"connection reset",
	}
	for _, marker := range transientMarkers {
		if contains(errStr, marker) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring (case-insensitive for SQLite errors)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCI(s, substr))
}

// containsCI is a simple case-insensitive contains check
func containsCI(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalFold is a simple case-insensitive string comparison
func equalFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if toLower(s[i]) != toLower(t[i]) {
			return false
		}
	}
	return true
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
