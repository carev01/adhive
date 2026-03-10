package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestDatabaseWithEntries extends TestDatabase for entry/tag tests
type TestDatabaseWithEntries struct {
	DB          *gorm.DB
	UserRepo    *repository.UserRepository
	SessionRepo *repository.SessionRepository
	EntryRepo   *repository.EntryRepository
	TagRepo     *repository.TagRepository
	TestUserID  string
	TestUser2ID string
}

func setupTestDB(t *testing.T) *TestDatabaseWithEntries {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	_ = db.AutoMigrate(
		&model.User{}, &model.Session{}, &model.CatalogEntry{},
		&model.Tag{}, &model.EntryTag{},
		&model.ArchiveRevision{}, &model.ArchiveAsset{},
		&model.Interaction{},
	)

	return &TestDatabaseWithEntries{
		DB:          db,
		UserRepo:    repository.NewUserRepository(db),
		SessionRepo: repository.NewSessionRepository(db),
		EntryRepo:   repository.NewEntryRepository(db),
		TagRepo:     repository.NewTagRepository(db),
	}
}

func setupTestRouter(td *TestDatabaseWithEntries) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Auth handler
	authHandler := handler.NewAuthHandler(td.UserRepo, td.SessionRepo)
	entryHandler := handler.NewEntryHandler(td.EntryRepo, td.TagRepo)
	tagHandler := handler.NewTagHandler(td.TagRepo)

	// Auth routes (public)
	r.POST("/api/v1/auth/register", authHandler.Register)
	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/logout", authHandler.Logout)

	// Protected routes with middleware
	authMiddleware := middleware.NewAuthMiddleware(td.SessionRepo)
	protected := r.Group("/api/v1")
	protected.Use(authMiddleware.Authenticate())
	{
		protected.GET("/entries", entryHandler.List)
		protected.POST("/entries", entryHandler.Create)
		protected.GET("/entries/:id", entryHandler.Get)
		protected.PUT("/entries/:id", entryHandler.Update)
		protected.DELETE("/entries/:id", entryHandler.Delete)

		protected.GET("/tags", tagHandler.List)
		protected.GET("/tags/:id", tagHandler.Get)
		protected.POST("/tags", tagHandler.Create)
		protected.PUT("/tags/:id", tagHandler.Update)
		protected.DELETE("/tags/:id", tagHandler.Delete)
		protected.POST("/entries/:id/tags", tagHandler.AddEntryTag)
		protected.DELETE("/entries/:id/tags/:tag_id", tagHandler.RemoveEntryTag)
	}

	return r
}

func createTestUser(td *TestDatabaseWithEntries, email string) string {
	user := &model.User{
		ID:           "user-" + email,
		Email:        email,
		PasswordHash: "$2a$10$dummy", // Invalid hash but works for test
		DisplayName:  email,
		IsActive:     true,
	}
	_ = td.UserRepo.Create(user)

	// Create session
	session := &model.Session{
		ID:        "session-" + email,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	_ = td.SessionRepo.Create(session)

	return session.ID
}

// ============ Entry API Tests ============

func TestEntry_Create(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	td.TestUserID = createTestUser(td, "entrytest@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: "session-entrytest@example.com"}

	// Create entry
	payload := handler.CreateEntryRequest{URL: "https://example.com/test"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp handler.EntryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.URL != "https://example.com/test" {
		t.Errorf("Expected URL https://example.com/test, got %s", resp.URL)
	}
	if resp.ArchiveStatus != string(model.ArchiveStatusPending) {
		t.Errorf("Expected status pending, got %s", resp.ArchiveStatus)
	}
}

func TestEntry_Create_InvalidURL(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	td.TestUserID = createTestUser(td, "invalidurl@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: "session-invalidurl@example.com"}

	// Try create entry with invalid URL
	payload := handler.CreateEntryRequest{URL: "not-a-url"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestEntry_Create_NoAuth(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)

	payload := handler.CreateEntryRequest{URL: "https://example.com/test"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestEntry_Create_DuplicateURL(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "duptest@example.com")

	// Create first entry
	entry := &model.CatalogEntry{
		ID:            "entry-dup",
		UserID:        "user-duptest@example.com",
		URL:           "https://example.com/duplicate",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	// Try to create duplicate entry with same URL
	payload := handler.CreateEntryRequest{URL: "https://example.com/duplicate"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for duplicate URL, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp handler.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Detail != "This ad is already in your catalog" {
		t.Errorf("Expected specific error message, got: %s", resp.Detail)
	}
}

func TestEntry_Create_SameURL_DifferentUser(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)

	// Create user 1 with an entry
	createTestUser(td, "user1@example.com")
	entry := &model.CatalogEntry{
		ID:            "entry-user1",
		UserID:        "user-user1@example.com",
		URL:           "https://example.com/shared",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	// Create user 2 and try to add same URL
	user2Session := createTestUser(td, "user2@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: user2Session}

	payload := handler.CreateEntryRequest{URL: "https://example.com/shared"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed - different user can have same URL
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 for different user with same URL, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestEntry_List(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "listtest@example.com")

	// Create some entries
	entry1 := &model.CatalogEntry{ID: "entry-1", UserID: "user-listtest@example.com", URL: "https://example.com/1", ArchiveStatus: model.ArchiveStatusPending}
	entry2 := &model.CatalogEntry{ID: "entry-2", UserID: "user-listtest@example.com", URL: "https://example.com/2", ArchiveStatus: model.ArchiveStatusSuccess}
	_ = td.EntryRepo.Create(context.Background(), entry1)
	_ = td.EntryRepo.Create(context.Background(), entry2)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp handler.EntryListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 2 {
		t.Errorf("Expected 2 entries, got %d", resp.Total)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("Expected 2 entries in list, got %d", len(resp.Entries))
	}
}

func TestEntry_List_Pagination(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "pagtest@example.com")

	// Create 25 entries
	for i := 1; i <= 25; i++ {
		entry := &model.CatalogEntry{
			ID:            "entry-" + string(rune(i)),
			UserID:        "user-pagtest@example.com",
			URL:           "https://example.com/" + string(rune(i)),
			ArchiveStatus: model.ArchiveStatusPending,
		}
		_ = td.EntryRepo.Create(context.Background(), entry)
	}

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	// Request first page with limit 10
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries?page=1&limit=10", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp handler.EntryListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 25 {
		t.Errorf("Expected total 25, got %d", resp.Total)
	}
	if len(resp.Entries) != 10 {
		t.Errorf("Expected 10 entries, got %d", len(resp.Entries))
	}
	if resp.Page != 1 {
		t.Errorf("Expected page 1, got %d", resp.Page)
	}
}

func TestEntry_List_Empty(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "emptytest@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp handler.EntryListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 0 {
		t.Errorf("Expected 0 entries, got %d", resp.Total)
	}
	if resp.Entries == nil {
		t.Error("Expected non-nil entries slice for empty list (empty slice preferred over null in JSON)")
	}
	if len(resp.Entries) != 0 {
		t.Errorf("Expected 0 entries in slice, got %d", len(resp.Entries))
	}
}

func TestEntry_Get(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "gettest@example.com")

	// Create entry
	entry := &model.CatalogEntry{
		ID:            "entry-get",
		UserID:        "user-gettest@example.com",
		URL:           "https://example.com/get",
		Title:         "Test Entry",
		ArchiveStatus: model.ArchiveStatusSuccess,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/entry-get", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp handler.EntryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Title != "Test Entry" {
		t.Errorf("Expected title 'Test Entry', got %s", resp.Title)
	}
}

func TestEntry_Get_NotFound(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "notfound@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/nonexistent", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestEntry_Get_OtherUserEntry(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "user1@example.com")

	// Create entry owned by different user
	entry := &model.CatalogEntry{
		ID:            "entry-other",
		UserID:        "user-other",
		URL:           "https://example.com/other",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/entry-other", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for other user's entry, got %d", w.Code)
	}
}

func TestEntry_Update(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "updatetest@example.com")

	// Create entry
	entry := &model.CatalogEntry{
		ID:            "entry-update",
		UserID:        "user-updatetest@example.com",
		URL:           "https://example.com/update",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	title := "Updated Title"
	desc := "Updated Description"
	payload := handler.UpdateEntryRequest{
		Title:       &title,
		Description: &desc,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/entries/entry-update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp handler.EntryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got %s", resp.Title)
	}
	if resp.Description != "Updated Description" {
		t.Errorf("Expected description 'Updated Description', got %s", resp.Description)
	}
}

func TestEntry_Update_NotFound(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "updatenotfound@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	title := "Test"
	payload := handler.UpdateEntryRequest{Title: &title}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/entries/nonexistent", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestEntry_Delete(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "deletetest@example.com")

	// Create entry
	entry := &model.CatalogEntry{
		ID:            "entry-delete",
		UserID:        "user-deletetest@example.com",
		URL:           "https://example.com/delete",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/entries/entry-delete", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", w.Code)
	}

	// Verify deleted
	_, err := td.EntryRepo.GetByID(td.DB.Statement.Context, "entry-delete")
	if err == nil {
		t.Error("Expected entry to be deleted")
	}
}

func TestEntry_Delete_NotFound(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "deletenotfound@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/entries/nonexistent", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

// ============ Tag API Tests ============

func TestTag_Create(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagcreate@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := handler.CreateTagRequest{Name: "TestTag", Color: "#FF5733"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Name != "TestTag" {
		t.Errorf("Expected name 'TestTag', got %s", resp.Name)
	}
	if resp.Color != "#FF5733" {
		t.Errorf("Expected color #FF5733, got %s", resp.Color)
	}
}

func TestTag_Create_DefaultColor(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagcolor@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := handler.CreateTagRequest{Name: "NoColorTag"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d", w.Code)
	}

	var resp handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Color != "#6B7280" {
		t.Errorf("Expected default color #6B7280, got %s", resp.Color)
	}
}

func TestTag_Create_InvalidColor(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "taginvalid@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := handler.CreateTagRequest{Name: "BadColor", Color: "red"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid color, got %d", w.Code)
	}
}

func TestTag_Create_NoName(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagnoname@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := handler.CreateTagRequest{Name: ""}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty name, got %d", w.Code)
	}
}

func TestTag_List(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "taglist@example.com")

	// Create tags
	tag1 := &model.Tag{ID: "tag-1", UserID: "user-taglist@example.com", Name: "Tag1", Color: "#FF0000"}
	tag2 := &model.Tag{ID: "tag-2", UserID: "user-taglist@example.com", Name: "Tag2", Color: "#00FF00"}
	_ = td.TagRepo.Create(tag1)
	_ = td.TagRepo.Create(tag2)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp []*handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(resp))
	}
}

func TestTag_List_Empty(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagempty@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp []*handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp) != 0 {
		t.Errorf("Expected 0 tags, got %d", len(resp))
	}
}

func TestTag_Get(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagget@example.com")

	tag := &model.Tag{ID: "tag-get", UserID: "user-tagget@example.com", Name: "GetTag", Color: "#0000FF"}
	_ = td.TagRepo.Create(tag)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/tag-get", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	var resp handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Name != "GetTag" {
		t.Errorf("Expected name 'GetTag', got %s", resp.Name)
	}
}

func TestTag_Get_NotFound(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "taggetnotfound@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/nonexistent", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestTag_Update(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagupdate@example.com")

	tag := &model.Tag{ID: "tag-update", UserID: "user-tagupdate@example.com", Name: "OldName", Color: "#FF0000"}
	_ = td.TagRepo.Create(tag)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	name := "NewName"
	color := "#00FFFF"
	payload := handler.UpdateTagRequest{Name: &name, Color: &color}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/tag-update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp handler.TagResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Name != "NewName" {
		t.Errorf("Expected name 'NewName', got %s", resp.Name)
	}
	if resp.Color != "#00FFFF" {
		t.Errorf("Expected color #00FFFF, got %s", resp.Color)
	}
}

func TestTag_Update_NotFound(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagupdatenotfound@example.com")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	name := "Test"
	payload := handler.UpdateTagRequest{Name: &name}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/nonexistent", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestTag_Delete(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "tagdelete@example.com")

	tag := &model.Tag{ID: "tag-delete", UserID: "user-tagdelete@example.com", Name: "DeleteTag", Color: "#FF0000"}
	_ = td.TagRepo.Create(tag)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/tag-delete", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", w.Code)
	}

	// Verify deleted
	_, err := td.TagRepo.FindByID("tag-delete")
	if err == nil {
		t.Error("Expected tag to be deleted")
	}
}

// ============ Entry-Tag Association Tests ============

func TestTag_AddEntryTag(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "assoctest@example.com")

	// Create entry and tag
	entry := &model.CatalogEntry{ID: "entry-assoc", UserID: "user-assoctest@example.com", URL: "https://example.com", ArchiveStatus: model.ArchiveStatusPending}
	tag := &model.Tag{ID: "tag-assoc", UserID: "user-assoctest@example.com", Name: "AssocTag", Color: "#FF0000"}
	_ = td.EntryRepo.Create(context.Background(), entry)
	_ = td.TagRepo.Create(tag)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := map[string]string{"tag_id": "tag-assoc"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/entry-assoc/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTag_AddEntryTag_InvalidTag(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "assoctest2@example.com")

	entry := &model.CatalogEntry{ID: "entry-assoc2", UserID: "user-assoctest2@example.com", URL: "https://example.com", ArchiveStatus: model.ArchiveStatusPending}
	_ = td.EntryRepo.Create(context.Background(), entry)

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	payload := map[string]string{"tag_id": "nonexistent"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/entry-assoc2/tags", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestTag_RemoveEntryTag(t *testing.T) {
	td := setupTestDB(t)
	router := setupTestRouter(td)
	sessionID := createTestUser(td, "assoctest3@example.com")

	entry := &model.CatalogEntry{ID: "entry-assoc3", UserID: "user-assoctest3@example.com", URL: "https://example.com", ArchiveStatus: model.ArchiveStatusPending}
	tag := &model.Tag{ID: "tag-assoc3", UserID: "user-assoctest3@example.com", Name: "AssocTag3", Color: "#FF0000"}
	_ = td.EntryRepo.Create(context.Background(), entry)
	_ = td.TagRepo.Create(tag)

	// Add the association
	_ = td.TagRepo.AddEntryTag("entry-assoc3", "tag-assoc3")

	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/entries/entry-assoc3/tags/tag-assoc3", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", w.Code)
	}
}

// ============ Entry Search Tests ============
