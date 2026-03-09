package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carev01/adhive/internal/auth"
	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestDatabase holds the test DB and repos for integration tests
type TestDatabase struct {
	DB         *gorm.DB
	UserRepo    *repository.UserRepository
	SessionRepo *repository.SessionRepository
}

func setupIntegrationDB(t *testing.T) *TestDatabase {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	_ = db.AutoMigrate(&model.User{}, &model.Session{})

	return &TestDatabase{
		DB:         db,
		UserRepo:    repository.NewUserRepository(db),
		SessionRepo: repository.NewSessionRepository(db),
	}
}

func setupIntegrationRouter(td *TestDatabase) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Auth handler
	authHandler := handler.NewAuthHandler(td.UserRepo, td.SessionRepo)

	// Auth routes
	r.POST("/api/v1/auth/register", authHandler.Register)
	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/logout", authHandler.Logout)

	// Protected routes with middleware
	authMiddleware := middleware.NewAuthMiddleware(td.SessionRepo)
	protected := r.Group("/api/v1/protected")
	protected.Use(authMiddleware.Authenticate())
	{
		protected.GET("/me", authHandler.Me)
		protected.GET("/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	}

	return r
}

// TestFullAuthFlow tests the complete authentication flow:
// 1. Register new user
// 2. Login
// 3. Access protected route
// 4. Logout
func TestFullAuthFlow(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Step 1: Register a new user
	registerPayload := handler.RegisterRequest{
		Email:       "flowtest@example.com",
		Password:   "SecureP@ss1",
		DisplayName: "Flow Test User",
	}
	registerBody, _ := json.Marshal(registerPayload)

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerW := httptest.NewRecorder()

	router.ServeHTTP(registerW, registerReq)

	if registerW.Code != http.StatusCreated {
		t.Fatalf("Register failed: expected %d, got %d. Body: %s", http.StatusCreated, registerW.Code, registerW.Body.String())
	}

	// Extract session cookie from registration
	registerCookies := registerW.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range registerCookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("No session cookie received after registration")
	}

	// Step 2: Login with the registered user
	loginPayload := handler.LoginRequest{
		Email:    "flowtest@example.com",
		Password: "SecureP@ss1",
	}
	loginBody, _ := json.Marshal(loginPayload)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()

	router.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("Login failed: expected %d, got %d. Body: %s", http.StatusOK, loginW.Code, loginW.Body.String())
	}

	// Step 3: Access protected route with session cookie
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/protected/me", nil)
	meReq.AddCookie(sessionCookie)
	meW := httptest.NewRecorder()

	router.ServeHTTP(meW, meReq)

	if meW.Code != http.StatusOK {
		t.Fatalf("Protected route access failed: expected %d, got %d. Body: %s", http.StatusOK, meW.Code, meW.Body.String())
	}

	// Verify response contains user data
	var meResp handler.UserResponse
	_ = json.Unmarshal(meW.Body.Bytes(), &meResp)

	if meResp.Email != "flowtest@example.com" {
		t.Errorf("Expected email flowtest@example.com, got %s", meResp.Email)
	}
	if meResp.DisplayName != "Flow Test User" {
		t.Errorf("Expected display name 'Flow Test User', got %s", meResp.DisplayName)
	}

	// Step 4: Logout
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutW := httptest.NewRecorder()

	router.ServeHTTP(logoutW, logoutReq)

	if logoutW.Code != http.StatusOK {
		t.Fatalf("Logout failed: expected %d, got %d", http.StatusOK, logoutW.Code)
	}

	// Verify session cookie is cleared
	logoutCookies := logoutW.Result().Cookies()
	var clearedCookie *http.Cookie
	for _, c := range logoutCookies {
		if c.Name == "session" {
			clearedCookie = c
			break
		}
	}
	if clearedCookie == nil || clearedCookie.MaxAge >= 0 {
		t.Error("Session cookie was not cleared after logout")
	}

	// Step 5: Verify access is denied after logout
	meAfterLogoutReq := httptest.NewRequest(http.MethodGet, "/api/v1/protected/me", nil)
	meAfterLogoutW := httptest.NewRecorder()

	router.ServeHTTP(meAfterLogoutW, meAfterLogoutReq)

	if meAfterLogoutW.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 after logout, got %d", meAfterLogoutW.Code)
	}
}

// TestAuthFlow_InvalidCredentials tests that invalid credentials are rejected
func TestAuthFlow_InvalidCredentials(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Try to login with non-existent user
	loginPayload := handler.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "AnyPassword1!",
	}
	loginBody, _ := json.Marshal(loginPayload)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()

	router.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for non-existent user, got %d", loginW.Code)
	}
}

// TestAuthFlow_ProtectedRouteWithoutAuth tests that protected routes require authentication
func TestAuthFlow_ProtectedRouteWithoutAuth(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Try to access protected route without session
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected/me", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 without session, got %d", w.Code)
	}
}

// TestAuthFlow_ExpiredSession tests that expired sessions are rejected
func TestAuthFlow_ExpiredSession(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Create user and manually create expired session
	hashedPassword, _ := auth.HashPassword("SecureP@ss1")
	user := &model.User{
		ID:           "test-user-id",
		Email:        "expired@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Expired Test",
		IsActive:     true,
	}
	_ = td.UserRepo.Create(user)

	// Create expired session in DB
	expiredSession := &model.Session{
		ID:        "expired-session-id",
		UserID:    user.ID,
		ExpiresAt: td.DB.NowFunc().Add(-24), // Expired 24 hours ago
	}
	_ = td.SessionRepo.Create(expiredSession)

	// Try to access protected route with expired session
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expired-session-id"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for expired session, got %d", w.Code)
	}
}

// TestAuthFlow_InactiveUser tests that inactive users cannot login
func TestAuthFlow_InactiveUser(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Create inactive user
	hashedPassword, _ := auth.HashPassword("SecureP@ss1")
	inactiveUser := &model.User{
		ID:           "inactive-user-id",
		Email:        "inactive@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Inactive User",
		IsActive:     false, // Inactive!
	}
	_ = td.UserRepo.Create(inactiveUser)

	// Try to login
	loginPayload := handler.LoginRequest{
		Email:    "inactive@example.com",
		Password: "SecureP@ss1",
	}
	loginBody, _ := json.Marshal(loginPayload)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()

	router.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for inactive user, got %d", loginW.Code)
	}
}

// TestAuthFlow_EmailConflict tests that duplicate emails are rejected
func TestAuthFlow_EmailConflict(t *testing.T) {
	td := setupIntegrationDB(t)
	router := setupIntegrationRouter(td)

	// Create existing user
	hashedPassword, _ := auth.HashPassword("SecureP@ss1")
	existingUser := &model.User{
		ID:           "existing-id",
		Email:        "duplicate@example.com",
		PasswordHash: hashedPassword,
		DisplayName:  "Existing User",
	}
	_ = td.UserRepo.Create(existingUser)

	// Try to register with same email
	registerPayload := handler.RegisterRequest{
		Email:    "duplicate@example.com",
		Password: "NewPassword1!",
	}
	registerBody, _ := json.Marshal(registerPayload)

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerW := httptest.NewRecorder()

	router.ServeHTTP(registerW, registerReq)

	if registerW.Code != http.StatusConflict {
		t.Errorf("Expected 409 Conflict for duplicate email, got %d", registerW.Code)
	}
}
