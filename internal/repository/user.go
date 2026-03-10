package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/carev01/adhive/internal/model"
)

// UserRepository handles user database operations
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

// FindByEmail finds a user by email
func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID finds a user by ID
func (r *UserRepository) FindByID(id string) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// SessionRepository handles session database operations
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new SessionRepository
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create creates a new session
func (r *SessionRepository) Create(session *model.Session) error {
	return r.db.Create(session).Error
}

// FindByID finds a session by ID
func (r *SessionRepository) FindByID(id string) (*model.Session, error) {
	var session model.Session
	err := r.db.Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// Delete deletes a session
func (r *SessionRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Session{}).Error
}

// DeleteExpired deletes all expired sessions
func (r *SessionRepository) DeleteExpired() error {
	return r.db.Where("expires_at < ?", time.Now()).Delete(&model.Session{}).Error
}

// GetUserBySession finds a user by session token
func (r *SessionRepository) GetUserBySession(sessionID string) (*model.User, error) {
	var session model.Session
	err := r.db.Where("id = ? AND expires_at > ?", sessionID, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}

	var user model.User
	err = r.db.Where("id = ?", session.UserID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
