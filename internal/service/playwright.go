package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PlaywrightConfig holds configuration for the Playwright scraper
type PlaywrightConfig struct {
	BrowserType         string // "chromium", "firefox", "webkit"
	Headless            bool
	Timeout             time.Duration
	UserAgent           string   // optional — leave empty to use JS scraper's auto-detected default
	ViewportWidth       int      // optional — 0 lets JS scraper pick from realistic resolution pool
	ViewportHeight      int      // optional — 0 lets JS scraper pick from realistic resolution pool
	HumanDomains        []string // domains requiring interactive/manual capture mode
	EnableManualCapture bool
}

// DefaultPlaywrightConfig returns default configuration.
// User-Agent and viewport are intentionally left empty so the JS scraper
// can derive them from the bundled Chromium version and a realistic
// resolution pool, avoiding version mismatches between Go and JS.
func DefaultPlaywrightConfig() PlaywrightConfig {
	// Load human domains from environment (comma-separated)
	var humanDomains []string
	if raw := os.Getenv("HUMAN_DOMAINS"); raw != "" {
		for _, d := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(d); t != "" {
				humanDomains = append(humanDomains, t)
			}
		}
	}

	// User-Agent override: leave empty by default so the JS scraper auto-detects
	// from the bundled Chromium version. Set USER_AGENT env to force a specific value.
	userAgent := os.Getenv("USER_AGENT")

	return PlaywrightConfig{
		BrowserType:         "chromium",
		Headless:            true,
		Timeout:             30 * time.Second,
		UserAgent:           userAgent,
		ViewportWidth:       0,
		ViewportHeight:      0,
		HumanDomains:        humanDomains,
		EnableManualCapture: true,
	}
}

// PlaywrightResult holds the result of a Playwright scrape
type PlaywrightResult struct {
	HTML                string             `json:"html"`
	StatusCode          int                `json:"status_code"`
	Screenshot          string             `json:"screenshot,omitempty"`
	Error               string             `json:"error,omitempty"`
	Headers             map[string]string  `json:"headers,omitempty"`
	FinalURL            string             `json:"final_url,omitempty"`
	ResourceURLs        []string           `json:"resource_urls,omitempty"`
	DOMAssetURLs        []string           `json:"dom_asset_urls,omitempty"`
	Cookies             []PlaywrightCookie `json:"cookies,omitempty"`
	RedirectChain       []string           `json:"redirect_chain,omitempty"`
	ChallengeDetected   bool               `json:"challenge_detected,omitempty"`
	ChallengeSignals    []string           `json:"challenge_signals,omitempty"`
	CrossDomainRedirect bool               `json:"cross_domain_redirect,omitempty"`
	SocialMediaRedirect bool               `json:"social_media_redirect,omitempty"`
	CaptureMode         string             `json:"capture_mode,omitempty"` // auto | manual
	TimeoutStage        string             `json:"timeout_stage,omitempty"`
	ErrorType           string             `json:"error_type,omitempty"`
}

// PlaywrightCookie is a serializable cookie exported from Playwright context.
type PlaywrightCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

// PlaywrightService provides browser-based scraping
type PlaywrightService struct {
	config PlaywrightConfig
	script string
}

// NewPlaywrightService creates a new Playwright service
func NewPlaywrightService(config PlaywrightConfig) *PlaywrightService {
	return &PlaywrightService{
		config: config,
		script: "playwright-scraper.js",
	}
}

// Scrape fetches a URL using Playwright
func (s *PlaywrightService) Scrape(ctx context.Context, url string, options map[string]interface{}) (*PlaywrightResult, error) {
	// Build config — only send fields that are explicitly set.
	// Omitted fields let the JS scraper use its own defaults (derived from
	// the bundled Chromium version), avoiding version mismatches.
	config := map[string]interface{}{
		"url":      url,
		"browser":  s.config.BrowserType,
		"headless": s.config.Headless,
		"timeout":  s.config.Timeout.Milliseconds(),
	}
	if s.config.UserAgent != "" {
		config["userAgent"] = s.config.UserAgent
	}
	if s.config.ViewportWidth > 0 {
		config["viewportWidth"] = s.config.ViewportWidth
	}
	if s.config.ViewportHeight > 0 {
		config["viewportHeight"] = s.config.ViewportHeight
	}

	// Manual capture mode for configured domains or explicit option.
	if s.shouldUseManualMode(url) {
		config["headless"] = false
		config["manualMode"] = true
		config["waitFor"] = "domcontentloaded"
		if _, ok := config["manualTimeoutMs"]; !ok {
			config["manualTimeoutMs"] = 120000
		}
	}

	// Merge options
	for k, v := range options {
		config[k] = v
	}

	// Convert to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Run Node.js script
	ctxTimeout := s.config.Timeout + 10*time.Second
	if v, ok := config["manualTimeoutMs"]; ok {
		switch t := v.(type) {
		case int:
			if t > 0 {
				ctxTimeout = time.Duration(t)*time.Millisecond + 15*time.Second
			}
		case float64:
			if t > 0 {
				ctxTimeout = time.Duration(int64(t))*time.Millisecond + 15*time.Second
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", s.script, string(configJSON))
	if dir := filepath.Dir(s.script); dir != "." {
		cmd.Dir = dir
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("playwright failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run playwright: %w", err)
	}

	// Parse result
	var result PlaywrightResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	if result.CaptureMode == "" {
		result.CaptureMode = "auto"
	}
	if result.Error != "" {
		return &result, fmt.Errorf("scrape error: %s", result.Error)
	}

	return &result, nil
}

// ScrapeWithRetry fetches with retry logic
func (s *PlaywrightService) ScrapeWithRetry(ctx context.Context, url string, maxRetries int) (*PlaywrightResult, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		options := map[string]interface{}{
			"waitFor":    "networkidle",
			"screenshot": true,
		}
		if s.shouldUseManualMode(url) {
			options["manualMode"] = true
			options["headless"] = false
			options["manualTimeoutMs"] = 180000
		}
		result, err := s.Scrape(ctx, url, options)

		if err == nil {
			return result, nil
		}

		lastErr = err
		log.Printf("Scrape attempt %d failed: %v", i+1, err)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (s *PlaywrightService) shouldUseManualMode(rawURL string) bool {
	// Feature must be globally enabled
	if !s.config.EnableManualCapture {
		return false
	}
	// If no domains configured, manual mode not auto‑triggered
	if len(s.config.HumanDomains) == 0 {
		return false
	}
	for _, domain := range s.config.HumanDomains {
		d := strings.TrimSpace(strings.ToLower(domain))
		if d == "" {
			continue
		}
		if strings.Contains(strings.ToLower(rawURL), d) {
			return true
		}
	}
	return false
}

// IsAvailable checks if Playwright is installed
func (s *PlaywrightService) IsAvailable() bool {
	// Check if Node.js is available
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}

	// Check if script exists
	if _, err := os.Stat(s.script); os.IsNotExist(err) {
		return false
	}

	return true
}

// InstallBrowsers installs Playwright browsers
func (s *PlaywrightService) InstallBrowsers() error {
	cmd := exec.Command("npx", "playwright", "install", s.config.BrowserType)
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_BROWSERS_PATH=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install browsers: %w\n%s", err, string(output))
	}

	return nil
}
