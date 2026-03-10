package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// HashPasswordForTest creates a bcrypt hash for testing purposes
func HashPasswordForTest(password string) (string, error) {
	return "", nil // Placeholder - will be implemented
}

// Helper functions for testing HTTP handlers

// Router creates a Gin router in test mode
func Router() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	return r
}

// MockRequest creates a test HTTP request
func MockRequest(method, path string, body interface{}) *http.Request {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			// In test context, panic or log - for now we'll return nil request
			// but better to handle properly
			panic(fmt.Errorf("failed to marshal request body: %w", err))
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// GetBody returns the body bytes from a ResponseRecorder
func GetBody(rec *httptest.ResponseRecorder) []byte {
	return rec.Body.Bytes()
}

// GetCode returns the status code from a ResponseRecorder
func GetCode(rec *httptest.ResponseRecorder) int {
	return rec.Code
}

// TestingT is an interface for test helpers
type TestingT interface {
	Errorf(format string, args ...interface{})
}
