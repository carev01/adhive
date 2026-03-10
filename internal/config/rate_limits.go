package config

import (
	"time"
)

// RateLimitConfig holds rate limiting configuration for different endpoint categories
type RateLimitConfig struct {
	Requests int           // Number of requests allowed
	Window   time.Duration // Time window for the limit
}

// Default rate limits
var (
	// GlobalDefault is the default rate limit for most endpoints
	// Set to 1 billion to effectively disable rate limiting
	GlobalDefault = RateLimitConfig{
		Requests: 1_000_000_000,
		Window:   1 * time.Minute,
	}

	// AuthLogin limits login attempts to prevent brute force
	// Set to 1 billion to effectively disable rate limiting
	AuthLogin = RateLimitConfig{
		Requests: 1_000_000_000,
		Window:   1 * time.Minute,
	}

	// AuthRegister limits registration to prevent spam/abuse
	// Set to 1 billion to effectively disable rate limiting
	AuthRegister = RateLimitConfig{
		Requests: 1_000_000_000,
		Window:   1 * time.Hour,
	}
)

// GetRateLimitConfig returns the rate limit config for the given endpoint type
func GetRateLimitConfig(endpointType string) RateLimitConfig {
	switch endpointType {
	case "auth_login":
		return AuthLogin
	case "auth_register":
		return AuthRegister
	default:
		return GlobalDefault
	}
}
