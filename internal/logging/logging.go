// Package logging provides structured logging for AdHive using slog (Go 1.21+).
//
// # Key Features
//
//   - Context propagation for trace correlation
//   - Operation logging with automatic timing
//   - Error logging with structured fields
//   - Default singleton logger with JSON output
//
// # Usage
//
//	logger := logging.Default()
//	ctx := context.WithValue(context.Background(), logging.ContextKeyTraceID, "abc-123")
//	logger.WithContext(ctx).Info("request received")
//
//	logger.LogOperation(ctx, "saveEntry", func() error {
//	    return saveToDB(entry)
//	})
package logging

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// ContextKey for request-scoped values used in logging context.
type ContextKey string

const (
	// ContextKeyTraceID is the context key for trace ID (request correlation).
	ContextKeyTraceID ContextKey = "trace_id"
	// ContextKeyUserID is the context key for user ID.
	ContextKeyUserID ContextKey = "user_id"
	// ContextKeyEntryID is the context key for entry ID (for entry-specific operations).
	ContextKeyEntryID ContextKey = "entry_id"
)

// Logger wraps slog.Logger with contextual logging utilities.
type Logger struct {
	*slog.Logger
}

// defaultLogger is the package-level default logger singleton.
var defaultLogger *Logger

func init() {
	// Initialize default logger with JSON handler outputting to stdout
	// Level is set to Info by default (can be changed via environment)
	level := slog.LevelInfo
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch envLevel {
		case "DEBUG":
			level = slog.LevelDebug
		case "WARN":
			level = slog.LevelWarn
		case "ERROR":
			level = slog.LevelError
		}
	}

	defaultLogger = &Logger{
		Logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
			// Add source location for debugging
			AddSource: true,
		})),
	}
}

// Default returns the default logger singleton.
func Default() *Logger {
	return defaultLogger
}

// SetDefault replaces the default logger with a custom one.
func SetDefault(l *Logger) {
	defaultLogger = l
}

// SetLevel changes the log level of the default logger.
func SetLevel(level slog.Level) {
	// This requires replacing the handler, but for simplicity we recreate the logger
	defaultLogger = &Logger{
		Logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       level,
			AddSource:   true,
		})),
	}
}

// WithContext returns a logger enriched with context values from the given context.
// It extracts trace_id, user_id, and entry_id from the context and adds them as
// structured attributes to the logger.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	var attrs []any

	if traceID := ctx.Value(ContextKeyTraceID); traceID != nil {
		if s, ok := traceID.(string); ok && s != "" {
			attrs = append(attrs, string(ContextKeyTraceID), s)
		}
	}
	if userID := ctx.Value(ContextKeyUserID); userID != nil {
		if s, ok := userID.(string); ok && s != "" {
			attrs = append(attrs, string(ContextKeyUserID), s)
		}
	}
	if entryID := ctx.Value(ContextKeyEntryID); entryID != nil {
		if s, ok := entryID.(string); ok && s != "" {
			attrs = append(attrs, string(ContextKeyEntryID), s)
		}
	}

	if len(attrs) == 0 {
		return l
	}

	return &Logger{
		Logger: l.Logger.With(attrs...),
	}
}

// With creates a new logger with additional attributes.
func (l *Logger) With(attrs ...slog.Attr) *Logger {
	var args []any
	for _, a := range attrs {
		args = append(args, a.Key, a.Value.Any())
	}
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// Warn logs a warning message with optional key-value pairs.
func (l *Logger) Warn(msg string, args ...any) {
	attrs := []any{}
	// Process args as key-value pairs
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			attrs = append(attrs, slog.Any(key, args[i+1]))
		}
	}
	l.Logger.Warn(msg, attrs...)
}

// Error logs an error message with the given error and optional key-value pairs.
// The error is logged as a structured attribute.
func (l *Logger) Error(err error, msg string, args ...any) {
	attrs := []any{slog.Any("error", err)}
	// Process args as key-value pairs
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			attrs = append(attrs, slog.Any(key, args[i+1]))
		}
	}
	l.Logger.Error(msg, attrs...)
}

// LogError logs an error with structured fields for detailed debugging.
// It uses WithContext to add trace correlation and includes the error along
// with any additional fields provided.
func (l *Logger) LogError(ctx context.Context, err error, message string, fields ...slog.Attr) {
	logger := l.WithContext(ctx)
	
	// Build args for Error method
	args := []any{}
	for _, f := range fields {
		args = append(args, f.Key, f.Value.Any())
	}
	
	logger.Error(err, message, args...)
}

// LogOperation logs the start, end (success or failure), and duration of an operation.
// It automatically measures execution time and logs appropriate messages.
//
// Example:
//
//	err := logger.LogOperation(ctx, "processEntry", func() error {
//	    return process(entry)
//	})
//	if err != nil {
//	    // Operation failed, already logged
//	}
func (l *Logger) LogOperation(ctx context.Context, operation string, fn func() error) error {
	logger := l.WithContext(ctx)

	logger.Info("operation started",
		slog.String("operation", operation),
	)

	start := time.Now()
	err := fn()
	duration := time.Since(start)

	if err != nil {
		logger.Error(err, "operation failed",
			slog.String("operation", operation),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
		return err
	}

	logger.Info("operation completed",
		slog.String("operation", operation),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)

	return nil
}

// WithTrace creates a new context with the trace ID set.
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ContextKeyTraceID, traceID)
}

// WithUser creates a new context with the user ID set.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// WithEntry creates a new context with the entry ID set.
func WithEntry(ctx context.Context, entryID string) context.Context {
	return context.WithValue(ctx, ContextKeyEntryID, entryID)
}
