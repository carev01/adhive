package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/carev01/adhive/internal/errors"
)

// ErrorResponse represents the standardized error response format.
type ErrorResponse struct {
	Type    string                 `json:"type"`
	Title   string                 `json:"title"`
	Status  int                    `json:"status"`
	Detail  string                 `json:"detail"`
	Code    string                 `json:"code,omitempty"`
	TraceID string                 `json:"trace_id,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// ErrorCodeRegistry maps ErrorCode to HTTP status codes.
var ErrorCodeRegistry = map[apperrors.ErrorCode]int{
	// Validation errors (400)
	apperrors.CodeInvalidInput:    http.StatusBadRequest,
	apperrors.CodeInvalidEmail:    http.StatusBadRequest,
	apperrors.CodeInvalidPassword: http.StatusBadRequest,
	apperrors.CodeMissingField:    http.StatusBadRequest,
	apperrors.CodeInvalidFormat:   http.StatusBadRequest,

	// Auth errors (401/403)
	apperrors.CodeUnauthorized:   http.StatusUnauthorized,
	apperrors.CodeSessionExpired: http.StatusUnauthorized,
	apperrors.CodeForbidden:      http.StatusForbidden,
	apperrors.CodeInvalidToken:   http.StatusUnauthorized,

	// Not found errors (404)
	apperrors.CodeNotFound:      http.StatusNotFound,
	apperrors.CodeEntryNotFound: http.StatusNotFound,
	apperrors.CodeTagNotFound:   http.StatusNotFound,
	apperrors.CodeUserNotFound:  http.StatusNotFound,

	// Conflict errors (409)
	apperrors.CodeDuplicateEntry: http.StatusConflict,
	apperrors.CodeDuplicateTag:   http.StatusConflict,
	apperrors.CodeDuplicateUser:  http.StatusConflict,

	// External service errors (502)
	apperrors.CodePlaywrightFailed: http.StatusBadGateway,
	apperrors.CodeArchiveFailed:    http.StatusBadGateway,
	apperrors.CodeThumbnailFailed:  http.StatusBadGateway,
	apperrors.CodeExternalService:  http.StatusBadGateway,

	// Transient errors (503)
	apperrors.CodeDatabaseBusy:     http.StatusServiceUnavailable,
	apperrors.CodeRateLimited:      http.StatusTooManyRequests,
	apperrors.CodeTimeout:          http.StatusGatewayTimeout,
	apperrors.CodeTemporaryFailure: http.StatusServiceUnavailable,

	// Internal errors (500) - default for unknown
}

// SendError sends a standardized error response.
// It checks if the error is an AppError and extracts appropriate HTTP status.
// If not an AppError, it returns a generic internal server error.
func SendError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	// Check if it's an AppError
	appErr, ok := apperrors.IsAppError(err)
	if ok {
		// Get HTTP status from registry or use AppError's HTTPStatus
		httpStatus := appErr.HTTPStatus
		if httpStatus == 0 {
			if status, found := ErrorCodeRegistry[appErr.Code]; found {
				httpStatus = status
			} else {
				httpStatus = http.StatusInternalServerError
			}
		}

		// Build error response
		response := ErrorResponse{
			Type:    "about:blank",
			Title:   string(appErr.Category),
			Status:  httpStatus,
			Detail:  appErr.Message,
			Code:    string(appErr.Code),
			TraceID: c.GetString("trace_id"),
		}

		// Add context if present
		if len(appErr.Context) > 0 {
			response.Context = appErr.Context
		}

		c.JSON(httpStatus, response)
		return
	}

	// Fallback: generic internal server error for unknown errors
	response := ErrorResponse{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: "An unexpected error occurred",
	}
	c.JSON(http.StatusInternalServerError, response)
}

// SendErrorWithStatus sends an error with a specific HTTP status code.
// Use this for errors that don't have an AppError but need specific status.
func SendErrorWithStatus(c *gin.Context, status int, title, detail string) {
	response := ErrorResponse{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	c.JSON(status, response)
}
