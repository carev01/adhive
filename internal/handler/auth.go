package handler

import (
	"net/http"
	"time"

	"github.com/carev01/adhive/internal/auth"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Request/Response DTOs
type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"display_name"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type ErrorResponse struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Detail  string `json:"detail"`
}

type AuthResponse struct {
	User UserResponse `json:"user"`
}

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	userRepo    *repository.UserRepository
	sessionRepo *repository.SessionRepository
	sessionTTL  time.Duration
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(userRepo *repository.UserRepository, sessionRepo *repository.SessionRepository) *AuthHandler {
	return &AuthHandler{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		sessionTTL:  7 * 24 * time.Hour, // 7 days
	}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	// Validate email format
	if err := auth.ValidateEmail(req.Email); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "invalid email format",
		})
		return
	}

	// Validate password strength
	if err := auth.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	// Check if user already exists
	existing, _ := h.userRepo.FindByEmail(req.Email)
	if existing != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Type:   "about:blank",
			Title:  "Conflict",
			Status: http.StatusConflict,
			Detail: "email already registered",
		})
		return
	}

	// Hash password
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Type:   "about:blank",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "failed to process password",
		})
		return
	}

	// Create user
	user := &model.User{
		ID:           uuid.New().String(),
		Email:        req.Email,
		PasswordHash: hash,
		DisplayName:  req.DisplayName,
		IsActive:     true,
	}

	if err := h.userRepo.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Type:   "about:blank",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "failed to create user",
		})
		return
	}

	// Create session
	session := &model.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(h.sessionTTL),
	}

	if err := h.sessionRepo.Create(session); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Type:   "about:blank",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "failed to create session",
		})
		return
	}

	// Set session cookie
	h.setSessionCookie(c, session.ID)

	c.JSON(http.StatusCreated, AuthResponse{
		User: UserResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			CreatedAt:   user.CreatedAt,
		},
	})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	// Find user
	user, err := h.userRepo.FindByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Type:   "about:blank",
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "invalid email or password",
		})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Type:   "about:blank",
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "invalid email or password",
		})
		return
	}

	// Check if user is active
	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Type:   "about:blank",
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "account is disabled",
		})
		return
	}

	// Create session
	session := &model.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(h.sessionTTL),
	}

	if err := h.sessionRepo.Create(session); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Type:   "about:blank",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "failed to create session",
		})
		return
	}

	// Set session cookie
	h.setSessionCookie(c, session.ID)

	c.JSON(http.StatusOK, AuthResponse{
		User: UserResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			CreatedAt:   user.CreatedAt,
		},
	})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	sessionID, err := c.Cookie("session")
	if err == nil && sessionID != "" {
		h.sessionRepo.Delete(sessionID)
	}

	// Clear session cookie
	c.SetCookie("session", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// Me handles GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	// User should be set by auth middleware
	userVal, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Type:   "about:blank",
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "not authenticated",
		})
		return
	}

	user := userVal.(*model.User)

	c.JSON(http.StatusOK, UserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		CreatedAt:   user.CreatedAt,
	})
}

// setSessionCookie sets the session cookie
func (h *AuthHandler) setSessionCookie(c *gin.Context, sessionID string) {
	c.SetCookie("session", sessionID, int(h.sessionTTL.Seconds()), "/", "", false, true)
}
