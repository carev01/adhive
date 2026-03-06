package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carev01/adhive/internal/auth"
	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	db.AutoMigrate(&model.User{}, &model.Session{})
	return db
}

func setupRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	authHandler := handler.NewAuthHandler(userRepo, sessionRepo)

	r.POST("/api/v1/auth/register", authHandler.Register)
	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/logout", authHandler.Logout)

	return r
}

func TestRegister_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	payload := handler.RegisterRequest{
		Email:       "test@example.com",
		Password:    "Test1234!",
		DisplayName: "Test User",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp handler.AuthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.User.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", resp.User.Email)
	}
	if resp.User.DisplayName != "Test User" {
		t.Errorf("expected display name Test User, got %s", resp.User.DisplayName)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	payload := handler.RegisterRequest{
		Email:       "invalid-email",
		Password:    "Test1234!",
		DisplayName: "Test User",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	payload := handler.RegisterRequest{
		Email:       "test2@example.com",
		Password:    "weak",
		DisplayName: "Test User",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRegister_EmailConflict(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create existing user
	hashedPassword, _ := auth.HashPassword("Test1234!")
	existingUser := &model.User{
		ID:           "existing-id",
		Email:        "conflict@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Existing User",
	}
	db.Create(existingUser)

	payload := handler.RegisterRequest{
		Email:       "conflict@example.com",
		Password:    "Test1234!",
		DisplayName: "New User",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create user with known password
	hashedPassword, _ := auth.HashPassword("Test1234!")
	user := &model.User{
		ID:           "user-id",
		Email:        "login@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Test User",
		IsActive:     true,
	}
	db.Create(user)

	payload := handler.LoginRequest{
		Email:    "login@example.com",
		Password: "Test1234!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Check session cookie is set
	cookie := w.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != "session" {
		t.Errorf("expected session cookie to be set")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	payload := handler.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "Test1234!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create user with known password
	hashedPassword, _ := auth.HashPassword("Test1234!")
	user := &model.User{
		ID:           "user-id",
		Email:        "wrongpass@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Test User",
		IsActive:     true,
	}
	db.Create(user)

	payload := handler.LoginRequest{
		Email:    "wrongpass@example.com",
		Password: "WrongPassword1!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create inactive user
	hashedPassword, _ := auth.HashPassword("Test1234!")
	user := &model.User{
		ID:           "user-id",
		Email:        "inactive@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Test User",
		IsActive:     false,
	}
	db.Create(user)

	payload := handler.LoginRequest{
		Email:    "inactive@example.com",
		Password: "Test1234!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestLogout(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check session cookie is cleared
	cookie := w.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != "session" || cookie[0].MaxAge >= 0 {
		t.Errorf("expected session cookie to be cleared")
	}
}
