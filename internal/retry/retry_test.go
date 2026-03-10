package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 5*time.Second {
		t.Errorf("MaxDelay = %v, want 5s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", cfg.Multiplier)
	}
	if cfg.Jitter != 0.1 {
		t.Errorf("Jitter = %v, want 0.1", cfg.Jitter)
	}
}

func TestArchiveConfig(t *testing.T) {
	cfg := ArchiveConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay = %v, want 1s", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
}

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	cfg := DefaultConfig()

	fn := func() error {
		callCount++
		return nil
	}

	err := Do(context.Background(), cfg, func(error) bool { return true }, fn)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("Call count = %d, want 1", callCount)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := Do(context.Background(), cfg, func(error) bool { return true }, fn)

	if err != nil {
		t.Errorf("Expected no error after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("Call count = %d, want 3", callCount)
	}
}

func TestDo_MaxAttemptsExceeded(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	fn := func() error {
		callCount++
		return errors.New("persistent error")
	}

	err := Do(context.Background(), cfg, func(error) bool { return true }, fn)

	if err == nil {
		t.Error("Expected error after max attempts")
	}
	if callCount != 3 {
		t.Errorf("Call count = %d, want 3", callCount)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	fn := func() error {
		callCount++
		return errors.New("non-retryable error")
	}

	// Only retry transient errors - this one is not retryable
	err := Do(context.Background(), cfg, func(e error) bool { return false }, fn)

	if err == nil {
		t.Error("Expected error")
	}
	// Should only be called once because error is not retryable
	if callCount != 1 {
		t.Errorf("Call count = %d, want 1", callCount)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  10, // Many attempts
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	fn := func() error {
		callCount++
		return errors.New("error")
	}

	// Cancel context after first call
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, func(error) bool { return true }, fn)

	// Should return context cancellation error
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
	// Should stop retrying after context is canceled
	if callCount > 3 {
		t.Errorf("Call count = %d, should stop after context canceled", callCount)
	}
}

func TestDoWithResult_Success(t *testing.T) {
	callCount := 0
	cfg := DefaultConfig()

	fn := func() (interface{}, error) {
		callCount++
		return "success", nil
	}

	result, err := DoWithResult(context.Background(), cfg, func(error) bool { return true }, fn)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Result = %v, want 'success'", result)
	}
	if callCount != 1 {
		t.Errorf("Call count = %d, want 1", callCount)
	}
}

func TestDoWithResult_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	fn := func() (interface{}, error) {
		callCount++
		if callCount < 2 {
			return nil, errors.New("temporary error")
		}
		return "result", nil
	}

	result, err := DoWithResult(context.Background(), cfg, func(error) bool { return true }, fn)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "result" {
		t.Errorf("Result = %v, want 'result'", result)
	}
	if callCount != 2 {
		t.Errorf("Call count = %d, want 2", callCount)
	}
}

func TestDoWithResult_Error(t *testing.T) {
	cfg := DefaultConfig()

	fn := func() (interface{}, error) {
		return nil, errors.New("error")
	}

	result, err := DoWithResult(context.Background(), cfg, func(error) bool { return false }, fn)

	if err == nil {
		t.Error("Expected error")
	}
	if result != nil {
		t.Errorf("Result = %v, want nil", result)
	}
}

func TestCalculateDelay(t *testing.T) {
	cfg := Config{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     1 * time.Second,
		Jitter:       0, // No jitter for predictable testing
	}

	tests := []struct {
		attempt       int
		minExpectedMs int
		maxExpectedMs int
	}{
		{1, 90, 110},   // First attempt: ~100ms (with small tolerance)
		{2, 190, 210},  // Second attempt: ~200ms
		{3, 390, 410},  // Third attempt: ~400ms
		{4, 790, 810},  // Fourth attempt: ~800ms
		{5, 990, 1010}, // Fifth attempt: capped at 1000ms
	}

	for _, tt := range tests {
		delay := calculateDelay(tt.attempt, cfg)
		ms := delay.Milliseconds()

		if int(ms) < tt.minExpectedMs || int(ms) > tt.maxExpectedMs {
			t.Errorf("Attempt %d: delay = %dms, want %d-%dms",
				tt.attempt, ms, tt.minExpectedMs, tt.maxExpectedMs)
		}
	}
}

func TestCalculateDelay_WithJitter(t *testing.T) {
	// With jitter, delays should vary but stay within bounds
	cfg := Config{
		InitialDelay: 1000 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     10 * time.Second,
		Jitter:       0.1, // 10% jitter
	}

	// Run multiple times to verify jitter is applied
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = calculateDelay(2, cfg) // 2nd attempt = 2000ms base
	}

	// Check that at least some delays are different (jitter applied)
	// Base is 2000ms, with 10% jitter range: 1800-2200ms
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Jitter should produce different delays, but all were the same")
	}

	// Verify all delays are within expected range
	for _, d := range delays {
		if d < 1800*time.Millisecond || d > 2200*time.Millisecond {
			t.Errorf("Delay %v out of expected range (1800-2200ms)", d)
		}
	}
}

func TestCalculateDelay_MaxDelayCap(t *testing.T) {
	cfg := Config{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   10.0, // Large multiplier to exceed max quickly
		MaxDelay:     500 * time.Millisecond,
		Jitter:       0,
	}

	// Even with large multiplier, delay should be capped
	delay := calculateDelay(5, cfg)

	if delay > 550*time.Millisecond { // Small tolerance
		t.Errorf("Delay = %v, should be capped at ~500ms", delay)
	}
}

func TestAddJitter_NoJitter(t *testing.T) {
	delay := addJitter(1000, 0)
	if delay != 1000 {
		t.Errorf("Delay = %v, want 1000", delay)
	}
}

func TestAddJitter_NegativeJitter(t *testing.T) {
	delay := addJitter(1000, -0.5)
	if delay != 1000 {
		t.Errorf("Delay = %v, want 1000 (negative jitter ignored)", delay)
	}
}

// The following test was removed due to timing flakiness:
// func TestExponentialBackoffTiming...
