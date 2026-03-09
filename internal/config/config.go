package config

import (
	"os"
	"strconv"
	"time"
)

// AppConfig holds all application configuration
type AppConfig struct {
	// Server
	Port string

	// Data Directory (unified)
	DataDir string

	// Database (backwards compatible)
	DBPath string

	// Storage (backwards compatible)
	StorageDir string

	// Rate Limiting
	RateLimit       int
	RateLimitWindow time.Duration

	// Security
	SessionSecret string

	// CORS
	CORSAllowedOrigins string

	// CSRF
	CSRFEnabled   bool
	CSRFSecure    bool
	CSRFSameSite  string

	// Playwright
	UserAgent    string
	HumanDomains string

	// Logging
	LogLevel string
	Debug    bool
}

// LoadAppConfig loads configuration from environment variables
// Uses DATA_DIR for unified storage, with backwards compatibility for DB_PATH and STORAGE_DIR
func LoadAppConfig() *AppConfig {
	return &AppConfig{
		// Server
		Port: getEnvOrDefault("PORT", "8080"),

		// Data Directory
		DataDir: os.Getenv("DATA_DIR"),

		// Database
		DBPath: GetDBPath(),

		// Storage
		StorageDir: getEnvOrDefault("STORAGE_DIR", "./data"),

		// Rate Limiting
		RateLimit:       getEnvIntOrDefault("RATE_LIMIT", 100),
		RateLimitWindow: getEnvDurationOrDefault("RATE_LIMIT_WINDOW", time.Minute),

		// Security
		SessionSecret: os.Getenv("SESSION_SECRET"),

		// CORS
		CORSAllowedOrigins: os.Getenv("CORS_ALLOWED_ORIGINS"),

		// CSRF
		CSRFEnabled:  os.Getenv("CSRF_ENABLED") == "true",
		CSRFSecure:   os.Getenv("CSRF_SECURE") == "true",
		CSRFSameSite: getEnvOrDefault("CSRF_SAME_SITE", "Lax"),

		// Playwright
		UserAgent:    os.Getenv("USER_AGENT"),
		HumanDomains: os.Getenv("HUMAN_DOMAINS"),

		// Logging
		LogLevel: getEnvOrDefault("LOG_LEVEL", "info"),
		Debug:    os.Getenv("DEBUG") == "true",
	}
}

// getEnvOrDefault returns environment variable or default
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvIntOrDefault returns environment variable as int or default
func getEnvIntOrDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

// getEnvDurationOrDefault returns environment variable as duration or default
func getEnvDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

// IsProduction checks if running in production mode
func (c *AppConfig) IsProduction() bool {
	return c.CSRFEnabled || c.CSRFSecure
}
