package degradation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	
	// Default mode should be full
	if m.GetMode(FeatureArchive) != ModeFull {
		t.Errorf("Default mode = %v, want %v", m.GetMode(FeatureArchive), ModeFull)
	}
}

func TestManager_SetMode(t *testing.T) {
	m := NewManager()
	
	m.SetMode(FeatureArchive, ModeDegraded)
	
	if m.GetMode(FeatureArchive) != ModeDegraded {
		t.Errorf("GetMode() = %v, want %v", m.GetMode(FeatureArchive), ModeDegraded)
	}
	
	m.SetMode(FeatureArchive, ModeDisabled)
	
	if m.GetMode(FeatureArchive) != ModeDisabled {
		t.Errorf("GetMode() = %v, want %v", m.GetMode(FeatureArchive), ModeDisabled)
	}
}

func TestManager_GetMode_DefaultMode(t *testing.T) {
	m := NewManager()
	
	// Unset feature should return default (full)
	mode := m.GetMode(FeatureThumbnail)
	if mode != ModeFull {
		t.Errorf("GetMode() = %v, want %v (default)", mode, ModeFull)
	}
}

func TestManager_IsAvailable(t *testing.T) {
	m := NewManager()
	
	// Default: all features available
	if !m.IsAvailable(FeatureArchive) {
		t.Error("IsAvailable() should be true for full mode")
	}
	
	m.SetMode(FeatureArchive, ModeDegraded)
	if !m.IsAvailable(FeatureArchive) {
		t.Error("IsAvailable() should be true for degraded mode")
	}
	
	m.SetMode(FeatureArchive, ModeDisabled)
	if m.IsAvailable(FeatureArchive) {
		t.Error("IsAvailable() should be false for disabled mode")
	}
}

func TestManager_CanUse(t *testing.T) {
	m := NewManager()
	
	// Full mode
	if !m.CanUse(FeatureArchive) {
		t.Error("CanUse() should be true for full mode")
	}
	
	// Degraded mode
	m.SetMode(FeatureArchive, ModeDegraded)
	if !m.CanUse(FeatureArchive) {
		t.Error("CanUse() should be true for degraded mode")
	}
	
	// Disabled mode
	m.SetMode(FeatureArchive, ModeDisabled)
	if m.CanUse(FeatureArchive) {
		t.Error("CanUse() should be false for disabled mode")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	
	// Concurrent reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				m.SetMode(FeatureArchive, ModeDegraded)
			} else {
				m.GetMode(FeatureArchive)
			}
		}(i)
	}
	
	wg.Wait()
}

func TestDegradedFallback_ModeFull(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeFull)
	
	callCount := 0
	primary := func() error {
		callCount++
		return nil
	}
	fallback := func() error {
		return errors.New("fallback should not be called")
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("Primary called %d times, want 1", callCount)
	}
}

func TestDegradedFallback_ModeFull_Error(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeFull)
	
	primary := func() error {
		return errors.New("primary error")
	}
	fallbackCalled := false
	fallback := func() error {
		fallbackCalled = true
		return nil
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err == nil {
		t.Error("Expected error from primary")
	}
	if fallbackCalled {
		t.Error("Fallback should not be called in full mode")
	}
}

func TestDegradedFallback_ModeDegraded_Success(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeDegraded)
	
	primaryCalled := false
	primary := func() error {
		primaryCalled = true
		return nil
	}
	fallbackCalled := false
	fallback := func() error {
		fallbackCalled = true
		return nil
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !primaryCalled {
		t.Error("Primary should be called")
	}
	if fallbackCalled {
		t.Error("Fallback should not be called when primary succeeds")
	}
}

func TestDegradedFallback_ModeDegraded_Fallback(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeDegraded)
	
	primary := func() error {
		return errors.New("primary error")
	}
	fallbackCalled := false
	fallback := func() error {
		fallbackCalled = true
		return nil
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error (fallback succeeded), got %v", err)
	}
	if !fallbackCalled {
		t.Error("Fallback should be called when primary fails")
	}
}

func TestDegradedFallback_ModeDisabled(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeDisabled)
	
	primaryCalled := false
	primary := func() error {
		primaryCalled = true
		return nil
	}
	fallbackCalled := false
	fallback := func() error {
		fallbackCalled = true
		return nil
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if primaryCalled {
		t.Error("Primary should not be called in disabled mode")
	}
	if !fallbackCalled {
		t.Error("Fallback should be called in disabled mode")
	}
}

func TestDegradedFallback_ModeDisabled_NoFallback(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeDisabled)
	
	primaryCalled := false
	primary := func() error {
		primaryCalled = true
		return nil
	}
	
	err := DegradedFallback(context.Background(), m, FeatureArchive, primary, nil)
	
	if err == nil {
		t.Error("Expected error when no fallback available in disabled mode")
	}
	if primaryCalled {
		t.Error("Primary should not be called")
	}
}

func TestDegradedFallbackWithResult_ModeFull(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeFull)
	
	primary := func() (interface{}, error) {
		return "result", nil
	}
	fallback := func() (interface{}, error) {
		return "fallback", nil
	}
	
	result, err := DegradedFallbackWithResult(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "result" {
		t.Errorf("Result = %v, want 'result'", result)
	}
}

func TestDegradedFallbackWithResult_ModeDegraded_Fallback(t *testing.T) {
	m := NewManager()
	m.SetMode(FeatureArchive, ModeDegraded)
	
	primary := func() (interface{}, error) {
		return nil, errors.New("error")
	}
	fallback := func() (interface{}, error) {
		return "fallback result", nil
	}
	
	result, err := DegradedFallbackWithResult(context.Background(), m, FeatureArchive, primary, fallback)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "fallback result" {
		t.Errorf("Result = %v, want 'fallback result'", result)
	}
}

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	
	if cb.maxFailures != 3 {
		t.Errorf("maxFailures = %d, want 3", cb.maxFailures)
	}
	if cb.state != CircuitClosed {
		t.Errorf("Initial state = %v, want %v", cb.state, CircuitClosed)
	}
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	
	if cb.GetState() != CircuitClosed {
		t.Errorf("State = %v, want %v", cb.GetState(), CircuitClosed)
	}
}

func TestCircuitBreaker_Call_Success(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	
	fn := func() error { return nil }
	
	err := cb.Call(fn)
	
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("State = %v, want %v", cb.GetState(), CircuitClosed)
	}
}

func TestCircuitBreaker_Call_Failure_OpensCircuit(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)
	
	fn := func() error { return errors.New("error") }
	
	// First failure
	err := cb.Call(fn)
	if err == nil {
		t.Error("Expected error")
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("State after 1 failure = %v, want %v", cb.GetState(), CircuitClosed)
	}
	
	// Second failure - should open circuit
	err = cb.Call(fn)
	if err == nil {
		t.Error("Expected error")
	}
	if cb.GetState() != CircuitOpen {
		t.Errorf("State after 2 failures = %v, want %v", cb.GetState(), CircuitOpen)
	}
}

func TestCircuitBreaker_Call_OpenCircuit_FastFail(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Second)
	
	// Open the circuit
	cb.Call(func() error { return errors.New("error") })
	
	if cb.GetState() != CircuitOpen {
		t.Fatalf("State = %v, want %v", cb.GetState(), CircuitOpen)
	}
	
	// Next call should fail fast with ErrCircuitOpen
	fn := func() error { return nil }
	err := cb.Call(fn)
	
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_Call_OpenToHalfOpen_Timeout(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)
	
	// Open the circuit
	cb.Call(func() error { return errors.New("error") })
	
	if cb.GetState() != CircuitOpen {
		t.Fatalf("State = %v, want %v", cb.GetState(), CircuitOpen)
	}
	
	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)
	
	// Next call should transition to half-open
	fn := func() error { return nil }
	err := cb.Call(fn)
	
	// Should not get ErrCircuitOpen because it transitions to half-open
	if errors.Is(err, ErrCircuitOpen) {
		t.Error("Should transition to half-open after timeout")
	}
	if cb.GetState() != CircuitClosed {
		t.Errorf("State after successful call = %v, want %v", cb.GetState(), CircuitClosed)
	}
}

func TestCircuitBreaker_RecordFailure(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Second)
	
	cb.recordFailure()
	if cb.failures != 1 {
		t.Errorf("failures = %d, want 1", cb.failures)
	}
	
	cb.recordFailure()
	if cb.failures != 2 {
		t.Errorf("failures = %d, want 2", cb.failures)
	}
}

func TestCircuitBreaker_RecordFailure_OpensCircuit(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)
	
	cb.recordFailure()
	if cb.state != CircuitClosed {
		t.Errorf("State = %v, want %v", cb.state, CircuitClosed)
	}
	
	cb.recordFailure()
	if cb.state != CircuitOpen {
		t.Errorf("State = %v, want %v after max failures", cb.state, CircuitOpen)
	}
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)
	
	cb.recordFailure()
	cb.recordFailure()
	
	cb.recordSuccess()
	
	if cb.failures != 0 {
		t.Errorf("failures = %d, want 0", cb.failures)
	}
	if cb.state != CircuitClosed {
		t.Errorf("State = %v, want %v", cb.state, CircuitClosed)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)
	
	// Open the circuit
	cb.Call(func() error { return errors.New("error") })
	cb.Call(func() error { return errors.New("error") })
	
	if cb.GetState() != CircuitOpen {
		t.Fatalf("State = %v, want %v", cb.GetState(), CircuitOpen)
	}
	
	// Manual reset
	cb.Reset()
	
	if cb.GetState() != CircuitClosed {
		t.Errorf("State = %v, want %v after reset", cb.GetState(), CircuitClosed)
	}
	if cb.failures != 0 {
		t.Errorf("failures = %d, want 0 after reset", cb.failures)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(10, time.Second)
	
	var wg sync.WaitGroup
	successCount := 0
	errorCount := 0
	
	// Simulate concurrent calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Call(func() error {
				if i%2 == 0 {
					return nil
				}
				return errors.New("error")
			})
			if err == nil {
				successCount++
			} else {
				errorCount++
			}
		}()
	}
	
	wg.Wait()
	
	// Just verify no race conditions - actual counts may vary
	t.Logf("Success: %d, Errors: %d", successCount, errorCount)
}

func TestErrCircuitOpen(t *testing.T) {
	// Verify ErrCircuitOpen is a proper error
	if ErrCircuitOpen == nil {
		t.Error("ErrCircuitOpen should not be nil")
	}
	
	// Verify errors.Is works
	err := ErrCircuitOpen
	if !errors.Is(err, ErrCircuitOpen) {
		t.Error("errors.Is should return true for ErrCircuitOpen")
	}
}
