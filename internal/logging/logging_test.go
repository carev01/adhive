package logging

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextKeys(t *testing.T) {
	assert.Equal(t, ContextKey("trace_id"), ContextKeyTraceID)
	assert.Equal(t, ContextKey("user_id"), ContextKeyUserID)
	assert.Equal(t, ContextKey("entry_id"), ContextKeyEntryID)
}

func TestWithTrace(t *testing.T) {
	ctx := context.Background()
	traceID := "test-trace-123"
	
	ctx = WithTrace(ctx, traceID)
	
	result := ctx.Value(ContextKeyTraceID)
	assert.Equal(t, traceID, result)
}

func TestWithUser(t *testing.T) {
	ctx := context.Background()
	userID := "user-456"
	
	ctx = WithUser(ctx, userID)
	
	result := ctx.Value(ContextKeyUserID)
	assert.Equal(t, userID, result)
}

func TestWithEntry(t *testing.T) {
	ctx := context.Background()
	entryID := "entry-789"
	
	ctx = WithEntry(ctx, entryID)
	
	result := ctx.Value(ContextKeyEntryID)
	assert.Equal(t, entryID, result)
}

func TestWithContext_EmptyContext(t *testing.T) {
	logger := Default()
	ctx := context.Background()
	
	// Should not panic with empty context
	result := logger.WithContext(ctx)
	assert.NotNil(t, result)
}

func TestWithContext_WithTraceID(t *testing.T) {
	logger := Default()
	ctx := WithTrace(context.Background(), "trace-abc")
	
	result := logger.WithContext(ctx)
	assert.NotNil(t, result)
}

func TestWithContext_WithMultipleValues(t *testing.T) {
	logger := Default()
	ctx := context.Background()
	ctx = WithTrace(ctx, "trace-123")
	ctx = WithUser(ctx, "user-456")
	ctx = WithEntry(ctx, "entry-789")
	
	// Should not panic with multiple context values
	result := logger.WithContext(ctx)
	assert.NotNil(t, result)
}

func TestLogger_With(t *testing.T) {
	logger := Default()
	
	result := logger.With(slog.String("key", "value"))
	assert.NotNil(t, result)
}

func TestLogger_Error(t *testing.T) {
	logger := Default()
	testErr := errors.New("test error")
	
	// Should not panic when logging an error
	logger.Error(testErr, "test message", "detail", "some detail")
}

func TestLogger_LogError(t *testing.T) {
	logger := Default()
	ctx := WithTrace(context.Background(), "trace-xyz")
	testErr := errors.New("test error")
	
	// Should not panic
	logger.LogError(ctx, testErr, "operation failed", 
		slog.String("operation", "test-op"),
	)
}

func TestLogger_LogOperation_Success(t *testing.T) {
	logger := Default()
	ctx := WithTrace(context.Background(), "trace-op")
	
	err := logger.LogOperation(ctx, "testOperation", func() error {
		return nil
	})
	
	assert.NoError(t, err)
}

func TestLogger_LogOperation_Failure(t *testing.T) {
	logger := Default()
	ctx := WithTrace(context.Background(), "trace-op-fail")
	testErr := errors.New("operation failed")
	
	err := logger.LogOperation(ctx, "failingOperation", func() error {
		return testErr
	})
	
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
}

func TestDefault_ReturnsSingleton(t *testing.T) {
	logger1 := Default()
	logger2 := Default()
	
	// Both calls should return the same instance
	assert.Same(t, logger1, logger2)
}
