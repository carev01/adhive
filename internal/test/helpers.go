package test

import (
	"bytes"
	"encoding/json"
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
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ResponseRecorder wraps httptest.ResponseRecorder for chaining
type ResponseRecorder struct {
	*httptest.ResponseRecorder
}

func NewRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

// Bind binds the response body to a struct
func (r *ResponseRecorder) Bind(v interface{}) error {
	return json.Unmarshal(r.Body.Bytes(), v)
}

// AssertStatus checks if the status code matches
func (r *ResponseRecorder) AssertStatus(t TestingT, expected int) {
	if r.Code != expected {
		t.Errorf("expected status %d, got %d. Body: %s", expected, r.Code, r.Body.String())
	}
}

// AssertJSON checks if the response contains expected JSON
func (r *ResponseRecorder) AssertJSON(t TestingT, expected string) {
	var expectedJSON, actualJSON map[string]interface{}
	json.Unmarshal([]byte(expected), &expectedJSON)
	json.Unmarshal(r.Body.Bytes(), &actualJSON)
	
	// Simple comparison - just check keys exist
	for key := range expectedJSON {
		if _, ok := actualJSON[key]; !ok {
			t.Errorf("expected key '%s' in response", key)
		}
	}
}

// TestingT is an interface for test helpers
type TestingT interface {
	Errorf(format string, args ...interface{})
}
