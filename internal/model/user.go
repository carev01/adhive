package model

import (
	"time"
)

// User represents a user account in the system
type User struct {
	ID           string    `json:"id" gorm:"primaryKey;type:text"`
	Email        string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	DisplayName  string    `json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	IsActive     bool      `json:"is_active" gorm:"not null;default:false"`
}

// TableName specifies the table name for GORM
func (User) TableName() string {
	return "users"
}

// Session represents an active user session
type Session struct {
	ID        string    `json:"id" gorm:"primaryKey;type:text"`
	UserID    string    `json:"user_id" gorm:"index;not null"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName specifies the table name for GORM
func (Session) TableName() string {
	return "sessions"
}
