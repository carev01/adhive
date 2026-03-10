// Package degradation provides graceful degradation and circuit breaker patterns for AdHive.
package degradation

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Feature represents a feature that can be degraded.
type Feature string

const (
	// FeatureArchive represents the archive functionality.
	FeatureArchive Feature = "archive"
	// FeatureThumbnail represents the thumbnail generation functionality.
	FeatureThumbnail Feature = "thumbnail"
	// FeaturePlaywright represents the Playwright browser automation.
	FeaturePlaywright Feature = "playwright"
	// FeatureFTS5 represents the full-text search functionality.
	FeatureFTS5 Feature = "fts5"
)

// Mode represents the operational mode of a feature.
type Mode string

const (
	// ModeFull represents full functionality.
	ModeFull Mode = "full"
	// ModeDegraded represents reduced functionality.
	ModeDegraded Mode = "degraded"
	// ModeDisabled represents disabled functionality.
	ModeDisabled Mode = "disabled"
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed represents a closed circuit (normal operation).
	CircuitClosed CircuitState = iota
	// CircuitOpen represents an open circuit (failing fast).
	CircuitOpen
	// CircuitHalfOpen represents a half-open circuit (testing recovery).
	CircuitHalfOpen
)

// Manager manages the operational modes of features.
type Manager struct {
	mu          sync.RWMutex
	modes       map[Feature]Mode
	defaultMode Mode
}

// NewManager creates a new degradation manager.
func NewManager() *Manager {
	return &Manager{
		modes:       make(map[Feature]Mode),
		defaultMode: ModeFull,
	}
}

// SetMode sets the operational mode for a feature.
func (m *Manager) SetMode(feature Feature, mode Mode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modes[feature] = mode
}

// GetMode gets the operational mode for a feature.
func (m *Manager) GetMode(feature Feature) Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if mode, ok := m.modes[feature]; ok {
		return mode
	}
	return m.defaultMode
}

// IsAvailable checks if a feature is available (not disabled).
func (m *Manager) IsAvailable(feature Feature) bool {
	return m.GetMode(feature) != ModeDisabled
}

// CanUse checks if a feature can be used (full or degraded mode).
func (m *Manager) CanUse(feature Feature) bool {
	mode := m.GetMode(feature)
	return mode == ModeFull || mode == ModeDegraded
}

// DegradedFallback executes a function with fallback support based on the feature's mode.
// - ModeFull: executes primary only
// - ModeDegraded: tries primary, falls back on failure
// - ModeDisabled: executes fallback only
func DegradedFallback(ctx context.Context, m *Manager, feature Feature, primary func() error, fallback func() error) error {
	mode := m.GetMode(feature)

	switch mode {
	case ModeFull:
		// Execute primary only, fail fast
		return primary()

	case ModeDegraded:
		// Try primary, fall back on failure
		err := primary()
		if err != nil {
			// Log degradation event (could be enhanced with logging package)
			if fallback != nil {
				return fallback()
			}
			return err
		}
		return nil

	case ModeDisabled:
		// Execute fallback only
		if fallback != nil {
			return fallback()
		}
		return errors.New("feature disabled and no fallback available")

	default:
		// Default to full mode
		return primary()
	}
}

// DegradedFallbackWithResult is like DegradedFallback but returns a result.
func DegradedFallbackWithResult(ctx context.Context, m *Manager, feature Feature, primary func() (interface{}, error), fallback func() (interface{}, error)) (interface{}, error) {
	mode := m.GetMode(feature)

	switch mode {
	case ModeFull:
		return primary()

	case ModeDegraded:
		result, err := primary()
		if err != nil {
			if fallback != nil {
				return fallback()
			}
			return nil, err
		}
		return result, nil

	case ModeDisabled:
		if fallback != nil {
			return fallback()
		}
		return nil, errors.New("feature disabled and no fallback available")

	default:
		return primary()
	}
}

// CircuitBreaker implements a circuit breaker pattern.
type CircuitBreaker struct {
	mu          sync.Mutex
	maxFailures int
	state       CircuitState
	failures    int
	resetAfter  time.Duration
	lastFailure time.Time
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(maxFailures int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: maxFailures,
		state:       CircuitClosed,
		resetAfter:  resetAfter,
	}
}

// Call executes the provided function if the circuit is closed.
// Returns ErrCircuitOpen if the circuit is open.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if circuit should transition from open to half-open
	if cb.state == CircuitOpen {
		if time.Since(cb.lastFailure) >= cb.resetAfter {
			cb.state = CircuitHalfOpen
		} else {
			return ErrCircuitOpen
		}
	}

	// Execute the function
	err := fn()

	// Record the result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// recordFailure increments the failure count and opens the circuit if threshold reached.
func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// recordSuccess resets the failure count and closes the circuit.
func (cb *CircuitBreaker) recordSuccess() {
	cb.failures = 0
	cb.state = CircuitClosed
}

// GetState returns the current state of the circuit breaker.
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = CircuitClosed
}
