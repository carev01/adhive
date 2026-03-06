package handler

import (
	"bytes"
	"mime/multipart"
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

func setupThumbnailHardeningRouter(t *testing.T) (*gin.Engine, *model.CatalogEntry, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	tmp, err := os.MkdirTemp("", "thumb-hardening")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	db, err := gorm.Open(sqlite.Open(filepath.Join(tmp, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	if err := db.AutoMigrate(&model.CatalogEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	entryRepo := repository.NewEntryRepository(db)
	entry := &model.CatalogEntry{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", URL: "https://example.com", ArchiveStatus: model.ArchiveStatusSuccess}
	if err := entryRepo.Create(t.Context(), entry); err != nil {
		t.Fatalf("create entry: %v", err)
	}

	st := &config.StorageConfig{BaseDir: tmp, ArchivesDir: filepath.Join(tmp, "archives"), ThumbDir: filepath.Join(tmp, "thumbnails")}
	if err := st.EnsureDirectories(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	fs := service.NewFileService(st)
	h := NewFileHandler(fs, entryRepo, nil)

	r.POST("/api/v1/files/thumbnails/:entryID", h.UploadThumbnail)
	r.GET("/api/v1/files/thumbnails/:entryID", h.GetThumbnail)
	r.GET("/api/v1/files/thumbnails/raw/*path", h.GetRawThumbnail)

	return r, entry, tmp
}

func TestUploadThumbnail_InvalidEntry_Returns4xxNoFsPath(t *testing.T) {
	r, _, _ := setupThumbnailHardeningRouter(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "thumb.png")
	_, _ = fw.Write([]byte("png"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/thumbnails/not-a-uuid", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("expected 4xx, got %d body=%s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("/home/")) || bytes.Contains(w.Body.Bytes(), []byte("\\")) {
		t.Fatalf("response leaked filesystem path: %s", w.Body.String())
	}
}

func TestGetRawThumbnail_Traversal_Returns4xx(t *testing.T) {
	r, _, _ := setupThumbnailHardeningRouter(t)
	cases := []string{
		"/api/v1/files/thumbnails/raw/../secret",
		"/api/v1/files/thumbnails/raw/%2e%2e/secret",
		"/api/v1/files/thumbnails/raw/..%2fsecret",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, u, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected 4xx got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetThumbnail_UnknownEntry_Returns404(t *testing.T) {
	r, _, _ := setupThumbnailHardeningRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/thumbnails/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetThumbnail_TraversalPayload_NoFilesystemLeak(t *testing.T) {
	r, _, _ := setupThumbnailHardeningRouter(t)
	cases := []string{
		"/api/v1/files/thumbnails/../test",
		"/api/v1/files/thumbnails/%2e%2e/test",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, u, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("expected 4xx got %d body=%s", w.Code, w.Body.String())
			}
			if bytes.Contains(w.Body.Bytes(), []byte("/home/")) || bytes.Contains(w.Body.Bytes(), []byte("C:\\")) {
				t.Fatalf("filesystem path leaked in response: %s", w.Body.String())
			}
		})
	}
}
