// Package retry provides retry logic with exponential backoff for AdHive.
package retry

import (
	"context"
	"math/rand"
	"time"
)

// Config holds the configuration for retry logic.
type Config struct {
	MaxAttempts  int           // Maximum number of attempts (default 3)
	InitialDelay time.Duration // Initial delay between retries (default 100ms)
	MaxDelay     time.Duration // Maximum delay cap (default 5s)
	Multiplier   float64       // Exponential multiplier (default 2.0)
	Jitter       float64       // Jitter factor (default 0.1)
}

// DefaultConfig returns the default retry configuration.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// ArchiveConfig returns the retry configuration for archive operations.
func ArchiveConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// Do executes the provided function with retry logic.
// It takes a context, config, isRetryable predicate, and the function to execute.
// Returns the last error after all attempts are exhausted.
func Do(ctx context.Context, cfg Config, isRetryable func(error) bool, fn func() error) error {
	var lastErr error
	
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
		
		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Check if we should retry
		if attempt >= cfg.MaxAttempts || !isRetryable(err) {
			return lastErr
		}
		
		// Calculate delay with exponential backoff
		delay := calculateDelay(attempt, cfg)
		
		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
	
	return lastErr
}

// DoWithResult executes the provided function with retry logic and returns a result.
// Similar to Do but returns both a result and an error.
func DoWithResult(ctx context.Context, cfg Config, isRetryable func(error) bool, fn func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		
		// Execute the function
		result, err := fn()
		if err == nil {
			return result, nil // Success
		}
		
		lastErr = err
		
		// Check if we should retry
		if attempt >= cfg.MaxAttempts || !isRetryable(err) {
			return nil, lastErr
		}
		
		// Calculate delay with exponential backoff
		delay := calculateDelay(attempt, cfg)
		
		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
	
	return nil, lastErr
}

// calculateDelay calculates the delay for a given attempt number.
func calculateDelay(attempt int, cfg Config) time.Duration {
	// Exponential backoff: InitialDelay * Multiplier^(attempt-1)
	delay := float64(cfg.InitialDelay)
	for i := 1; i < attempt; i++ {
		delay *= cfg.Multiplier
	}
	
	// Cap at MaxDelay
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}
	
	// Add jitter
	delay = addJitter(delay, cfg.Jitter)
	
	return time.Duration(delay)
}

// addJitter adds random variance to the delay.
// The jitter is ±jitter% of the delay.
func addJitter(delay float64, jitter float64) float64 {
	if jitter <= 0 {
		return delay
	}
	
	// Generate random factor between (1 - jitter) and (1 + jitter)
	jitterRange := jitter * 2
	randomFactor := rand.Float64()*jitterRange + (1 - jitter)
	
	return delay * randomFactor
}
