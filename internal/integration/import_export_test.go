package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
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

func setupImportExportDB(t *testing.T) *TestDatabaseWithEntries {
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

func setupImportExportRouter(td *TestDatabaseWithEntries) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	entryHandler := handler.NewEntryHandler(td.EntryRepo, td.TagRepo)
	importExportHandler := handler.NewImportExportHandler(td.EntryRepo, td.TagRepo)
	authMiddleware := middleware.NewAuthMiddleware(td.SessionRepo)

	protected := r.Group("/api/v1")
	protected.Use(authMiddleware.Authenticate())
	{
		protected.GET("/entries", entryHandler.List)
		protected.POST("/entries", entryHandler.Create)
		protected.POST("/entries/import", importExportHandler.Import)
		protected.GET("/entries/export", importExportHandler.Export)
		protected.GET("/entries/template", importExportHandler.Template)
	}

	return r
}

func createImportTestUser(td *TestDatabaseWithEntries, email string) string {
	user := &model.User{
		ID:           "user-" + email,
		Email:        email,
		PasswordHash: "$2a$10$dummy",
		DisplayName:  email,
		IsActive:     true,
	}
	_ = td.UserRepo.Create(user)

	session := &model.Session{
		ID:        "session-" + email,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	_ = td.SessionRepo.Create(session)

	return session.ID
}

func TestImport_CSV_Success(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importcsv@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	csvContent := "url,title,description,phone_number,location\nhttps://example.com/ad1,Ad One,Description one,555-0001,City A\nhttps://example.com/ad2,Ad Two,Description two,555-0002,City B"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.csv")
	_, _ = part.Write([]byte(csvContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	imported, ok := resp["imported_count"].(float64)
	if !ok || imported != 2 {
		t.Errorf("Expected imported_count 2, got %v", resp["imported_count"])
	}
}

func TestImport_JSON_Success(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importjson@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	jsonData := map[string]interface{}{
		"entries": []map[string]string{
			{"url": "https://example.com/j1", "title": "JSON Ad 1", "description": "Desc 1"},
			{"url": "https://example.com/j2", "title": "JSON Ad 2", "location": "City X"},
		},
	}
	jsonBytes, _ := json.Marshal(jsonData)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.json")
	_, _ = part.Write(jsonBytes)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	imported, ok := resp["imported_count"].(float64)
	if !ok || imported != 2 {
		t.Errorf("Expected imported_count 2, got %v", resp["imported_count"])
	}
}

func TestImport_CSV_InvalidURL(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importinvalid@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	csvContent := "url,title\nnot-a-url,Bad URL\nhttps://example.com/good,Good URL"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.csv")
	_, _ = part.Write([]byte(csvContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 (partial success), got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	errorCount, _ := resp["error_count"].(float64)
	if errorCount != 1 {
		t.Errorf("Expected 1 error, got %v", resp["error_count"])
	}
	imported, _ := resp["imported_count"].(float64)
	if imported != 1 {
		t.Errorf("Expected 1 imported, got %v", resp["imported_count"])
	}
}

func TestImport_CSV_MergeExisting(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importmerge@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	// Pre-create an entry with the same URL
	existingEntry := &model.CatalogEntry{
		ID:            "entry-existing",
		UserID:        "user-importmerge@example.com",
		URL:           "https://example.com/merge",
		Title:         "Original Title",
		Description:   "Original Description",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = td.EntryRepo.Create(context.Background(), existingEntry)

	// CSV that matches existing URL — should merge, not duplicate
	csvContent := "url,title,description\nhttps://example.com/merge,Updated Title,"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.csv")
	_, _ = part.Write([]byte(csvContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	updated, _ := resp["updated_count"].(float64)
	if updated != 1 {
		t.Errorf("Expected updated_count 1, got %v", resp["updated_count"])
	}

	// Verify entry was merged (not duplicated)
	var count int64
	td.DB.Model(&model.CatalogEntry{}).Where("url = ? AND user_id = ?", "https://example.com/merge", "user-importmerge@example.com").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 entry with this URL, got %d", count)
	}

	// Verify title was updated
	var entry model.CatalogEntry
	td.DB.Where("url = ? AND user_id = ?", "https://example.com/merge", "user-importmerge@example.com").First(&entry)
	if entry.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", entry.Title)
	}
	// Description should be preserved (merge: empty import field → keep existing)
	if entry.Description != "Original Description" {
		t.Errorf("Expected description 'Original Description' (preserved), got '%s'", entry.Description)
	}
}

func TestImport_CSV_DuplicateWithinFile(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importdup@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	csvContent := "url,title\nhttps://example.com/dup,Dup One\nhttps://example.com/dup,Dup Two"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.csv")
	_, _ = part.Write([]byte(csvContent))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	imported, _ := resp["imported_count"].(float64)
	if imported != 1 {
		t.Errorf("Expected imported_count 1, got %v", resp["imported_count"])
	}
	skipped, _ := resp["skipped_count"].(float64)
	if skipped != 1 {
		t.Errorf("Expected skipped_count 1, got %v", resp["skipped_count"])
	}
}

func TestImport_NoAuth(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestImport_UnsupportedFormat(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "importformat@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "import.xlsx")
	_, _ = part.Write([]byte("data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestExport_CSV(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "exportcsv@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	// Create some entries
	entry1 := &model.CatalogEntry{ID: "export-1", UserID: "user-exportcsv@example.com", URL: "https://example.com/1", Title: "Export 1", Description: "Desc 1", PhoneNumber: "555-1111", Location: "City 1", ArchiveStatus: model.ArchiveStatusPending}
	entry2 := &model.CatalogEntry{ID: "export-2", UserID: "user-exportcsv@example.com", URL: "https://example.com/2", Title: "Export 2", Description: "Desc 2", PhoneNumber: "555-2222", Location: "City 2", ArchiveStatus: model.ArchiveStatusPending}
	_ = td.EntryRepo.Create(context.Background(), entry1)
	_ = td.EntryRepo.Create(context.Background(), entry2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/export?format=csv", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Errorf("Expected Content-Type text/csv, got %s", contentType)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("url,title,description,phone_number,location")) {
		t.Error("CSV should contain header row with expected columns")
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("https://example.com/1")) {
		t.Error("CSV should contain entry URL")
	}
}

func TestExport_JSON(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "exportjson@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	entry1 := &model.CatalogEntry{ID: "exportj-1", UserID: "user-exportjson@example.com", URL: "https://example.com/j1", Title: "JSON Export 1", ArchiveStatus: model.ArchiveStatusPending}
	_ = td.EntryRepo.Create(context.Background(), entry1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/export?format=json", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	entries, ok := resp["entries"].([]interface{})
	if !ok || len(entries) != 1 {
		t.Errorf("Expected 1 entry in JSON export, got %v", resp["entries"])
	}
	count, _ := resp["count"].(float64)
	if count != 1 {
		t.Errorf("Expected count 1, got %v", resp["count"])
	}
}

func TestExport_NoAuth(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/export?format=csv", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestExport_InvalidFormat(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "exportformat@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/export?format=xml", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid format, got %d", w.Code)
	}
}

func TestTemplate_Download(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "template@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/template", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Errorf("Expected Content-Type text/csv, got %s", contentType)
	}

	contentDisposition := w.Header().Get("Content-Disposition")
	if contentDisposition != "attachment; filename=adhive-import-template.csv" {
		t.Errorf("Expected Content-Disposition with template filename, got %s", contentDisposition)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("url,title,description,phone_number,location")) {
		t.Error("Template should contain expected headers")
	}
}

func TestRoundTrip_CSV_Export_Import(t *testing.T) {
	td := setupImportExportDB(t)
	router := setupImportExportRouter(td)
	sessionID := createImportTestUser(td, "roundtrip@example.com")
	sessionCookie := &http.Cookie{Name: "session", Value: sessionID}

	// Create entries
	entry1 := &model.CatalogEntry{ID: "rt-1", UserID: "user-roundtrip@example.com", URL: "https://example.com/rt1", Title: "Round Trip 1", Description: "RT Desc 1", PhoneNumber: "555-1111", Location: "RT City 1", ArchiveStatus: model.ArchiveStatusPending}
	entry2 := &model.CatalogEntry{ID: "rt-2", UserID: "user-roundtrip@example.com", URL: "https://example.com/rt2", Title: "Round Trip 2", Description: "RT Desc 2", PhoneNumber: "555-2222", Location: "RT City 2", ArchiveStatus: model.ArchiveStatusPending}
	_ = td.EntryRepo.Create(context.Background(), entry1)
	_ = td.EntryRepo.Create(context.Background(), entry2)

	// Export to CSV
	req := httptest.NewRequest(http.MethodGet, "/api/v1/entries/export?format=csv", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Export failed: %d. Body: %s", w.Code, w.Body.String())
	}

	csvData := w.Body.Bytes()

	// Re-import the exported CSV
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "reimport.csv")
	_, _ = part.Write(csvData)
	writer.Close()

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/entries/import", body)
	req2.Header.Set("Content-Type", writer.FormDataContentType())
	req2.AddCookie(sessionCookie)
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Import failed: %d. Body: %s", w2.Code, w2.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)

	// Both entries should be updated (matched by URL), not newly imported
	updated, _ := resp["updated_count"].(float64)
	if updated != 2 {
		t.Errorf("Expected updated_count 2 (re-import of same data), got %v", resp["updated_count"])
	}
}