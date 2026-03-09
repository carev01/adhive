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
	GlobalDefault = RateLimitConfig{
		Requests: 100,
		Window:   1 * time.Minute,
	}

	// AuthLogin limits login attempts to prevent brute force
	AuthLogin = RateLimitConfig{
		Requests: 5,
		Window:   1 * time.Minute,
	}

	// AuthRegister limits registration to prevent spam/abuse
	AuthRegister = RateLimitConfig{
		Requests: 3,
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
