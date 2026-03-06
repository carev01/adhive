package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carev01/adhive/internal/config"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupFileTraversalRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	tmp, err := os.MkdirTemp("", "file-traversal")
	if err != nil {
		t.Fatalf("failed temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	dbFile := filepath.Join(tmp, "test.db")
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed db open: %v", err)
	}
	if err := db.AutoMigrate(&model.CatalogEntry{}); err != nil {
		t.Fatalf("failed migrate: %v", err)
	}

	st := &config.StorageConfig{
		BaseDir:     tmp,
		ArchivesDir: filepath.Join(tmp, "archives"),
		ThumbDir:    filepath.Join(tmp, "thumbnails"),
	}
	if err := st.EnsureDirectories(); err != nil {
		t.Fatalf("failed init storage dirs: %v", err)
	}

	entryRepo := repository.NewEntryRepository(db)
	entry := &model.CatalogEntry{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", URL: "https://example.com", ArchiveStatus: model.ArchiveStatusSuccess}
	if err := entryRepo.Create(t.Context(), entry); err != nil {
		t.Fatalf("failed create entry: %v", err)
	}

	fs := service.NewFileService(st)
	h := NewFileHandler(fs, entryRepo, nil)

	r.GET("/api/v1/files/archives/:entryID", h.ListArchives)
	r.GET("/api/v1/files/archive/:entryID/:revisionID/*path", h.GetArchive)
	return r, st.ArchivesDir
}

func TestTraversal_ListArchives_FailClosed(t *testing.T) {
	r, _ := setupFileTraversalRouter(t)
	cases := []string{
		"../etc",
		"..%2fetc",
		"%2e%2e/etc",
		"..%252fetc", // double-encoded
		"..\\windows",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/files/archives/"+tc, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected explicit 4xx for %q, got %d body=%s", tc, w.Code, w.Body.String())
			}
			if w.Body.Len() == 0 {
				t.Fatalf("expected explicit error body for %q", tc)
			}
		})
	}
}

func TestListArchives_UnknownEntry_Explicit4xx(t *testing.T) {
	r, _ := setupFileTraversalRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/archives/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown entry, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListArchives_NoFiles_Explicit4xx(t *testing.T) {
	r, _ := setupFileTraversalRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/archives/11111111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no files exist, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestTraversal_GetArchive_FailClosed(t *testing.T) {
	r, archivesDir := setupFileTraversalRouter(t)

	// create a valid revision manifest so failures are from traversal sanitization, not missing baseline
	revDir := filepath.Join(archivesDir, "11111111-1111-1111-1111-111111111111", "rev-0001")
	if err := os.MkdirAll(revDir, 0o755); err != nil {
		t.Fatalf("mkdir rev: %v", err)
	}
	manifest := `{"schema_version":"1.0","revision_id":"rev-0001","entry_id":"11111111-1111-1111-1111-111111111111","revision_no":1,"captured_at":"2026-02-26T22:00:00Z","engine":"playwright","base_url":"https://example.com","status":"success","stats":{"total_assets":0,"downloaded_assets":0,"missing_assets":0,"skipped_assets":0,"error_assets":0,"total_bytes":0},"diagnostics":{},"files":[]}`
	if err := os.WriteFile(filepath.Join(revDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cases := []struct {
		revision string
		path     string
	}{
		{"../rev-0001", "/index.html"},
		{"..%2frev-0001", "/index.html"},
		{"%2e%2e/rev-0001", "/index.html"},
		{"rev-0001", "/../secret.txt"},
		{"rev-0001", "/..%2fsecret.txt"},
		{"rev-0001", "/%2e%2e/secret.txt"},
		{"rev-0001", "/..%252fsecret.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.revision+"__"+tc.path, func(t *testing.T) {
			url := "/api/v1/files/archive/11111111-1111-1111-1111-111111111111/" + tc.revision + tc.path
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected explicit 4xx, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
