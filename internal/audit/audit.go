package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// EventType categories for audit events
type EventType string

const (
	EventTypeAuth    EventType = "auth"
	EventTypeSession EventType = "session"
	EventTypeEntry   EventType = "entry"
	EventTypeArchive EventType = "archive"
	EventTypeSecurity EventType = "security"
)

// Action types for audit events
type Action string

const (
	ActionLogin     Action = "login"
	ActionLogout    Action = "logout"
	ActionRegister  Action = "register"
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionDelete    Action = "delete"
	ActionAccess    Action = "access"
	ActionRateLimit Action = "rate_limit_exceeded"
	ActionCSRF      Action = "csrf_failure"
	ActionAuthFail  Action = "auth_failure"
)

// Event represents a single audit event
type Event struct {
	Timestamp   time.Time   `json:"timestamp"`
	Type        EventType   `json:"type"`
	Action      Action      `json:"action"`
	UserID      string      `json:"user_id,omitempty"`
	Email       string      `json:"email,omitempty"`
	IPAddress   string      `json:"ip_address"`
	UserAgent   string      `json:"user_agent,omitempty"`
	ResourceID  string      `json:"resource_id,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	Success     bool        `json:"success"`
	Detail      string      `json:"detail,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Logger handles audit event logging
type Logger struct {
	output   io.Writer
	mu       sync.Mutex
	enabled  bool
}

// DefaultLogger is the default audit logger
var DefaultLogger = &Logger{
	output:  os.Stdout,
	enabled: true,
}

// SetOutput sets the output writer for audit logs
func SetOutput(w io.Writer) {
	DefaultLogger.mu.Lock()
	defer DefaultLogger.mu.Unlock()
	DefaultLogger.output = w
}

// SetEnabled enables or disables audit logging
func SetEnabled(enabled bool) {
	DefaultLogger.mu.Lock()
	defer DefaultLogger.mu.Unlock()
	DefaultLogger.enabled = enabled
}

// Log writes an audit event to the output
func (l *Logger) Log(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.enabled {
		return
	}

	event.Timestamp = time.Now().UTC()
	
	// Encode as JSON
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("audit: failed to marshal event: %v", err)
		return
	}
	
	// Write to output
	fmt.Fprintln(l.output, string(data))
}

// Log logs an event using the default logger
func Log(event Event) {
	DefaultLogger.Log(event)
}

// AuthSuccess logs a successful authentication event
func AuthSuccess(action Action, userID, email, ip, userAgent string) {
	Log(Event{
		Type:       EventTypeAuth,
		Action:     action,
		UserID:     userID,
		Email:      email,
		IPAddress:  ip,
		UserAgent:  userAgent,
		Success:    true,
		Detail:     fmt.Sprintf("%s successful", action),
	})
}

// AuthFailure logs a failed authentication event
func AuthFailure(action Action, email, ip, userAgent, detail string) {
	Log(Event{
		Type:       EventTypeAuth,
		Action:     ActionAuthFail,
		Email:      email,
		IPAddress:  ip,
		UserAgent:  userAgent,
		Success:    false,
		Detail:     detail,
	})
}

// SessionEvent logs a session-related event
func SessionEvent(action Action, userID, ip string, success bool) {
	Log(Event{
		Type:      EventTypeSession,
		Action:    action,
		UserID:    userID,
		IPAddress: ip,
		Success:   success,
	})
}

// EntryEvent logs an entry operation
func EntryEvent(action Action, userID, entryID, ip string, success bool) {
	Log(Event{
		Type:         EventTypeEntry,
		Action:       action,
		UserID:       userID,
		ResourceID:   entryID,
		ResourceType: "entry",
		IPAddress:    ip,
		Success:      success,
	})
}

// ArchiveEvent logs an archive operation
func ArchiveEvent(action Action, userID, entryID, ip string, success bool) {
	Log(Event{
		Type:         EventTypeArchive,
		Action:       action,
		UserID:       userID,
		ResourceID:   entryID,
		ResourceType: "archive",
		IPAddress:    ip,
		Success:      success,
	})
}

// SecurityEvent logs a security event (rate limit, CSRF, etc.)
func SecurityEvent(action Action, ip, userAgent, detail string) {
	Log(Event{
		Type:      EventTypeSecurity,
		Action:    action,
		IPAddress: ip,
		UserAgent: userAgent,
		Success:   false,
		Detail:    detail,
	})
}

// InitFromEnv initializes audit logging from environment variables
func InitFromEnv() {
	// Check if audit logging is enabled
	auditEnabled := os.Getenv("AUDIT_LOG_ENABLED")
	if auditEnabled == "false" {
		SetEnabled(false)
	}

	// Allow setting output file via environment variable
	auditOutput := os.Getenv("AUDIT_LOG_FILE")
	if auditOutput != "" {
		f, err := os.OpenFile(auditOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("audit: failed to open output file: %v", err)
		} else {
			SetOutput(f)
		}
	}
}