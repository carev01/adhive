package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carev01/adhive/internal/logging"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupWorkerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	_ = db.AutoMigrate(&model.CatalogEntry{})
	return db
}

// Mock HTTP server for testing
func createMockServer(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestArchiveWorker_NewArchiveWorker(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)

	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	if worker == nil {
		t.Fatal("Expected worker to not be nil")
	}
	if worker.entryRepo == nil {
		t.Error("Expected entryRepo to be set")
	}
	if worker.httpClient == nil {
		t.Error("Expected httpClient to be set")
	}
}

func TestArchiveWorker_QueueJob(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)

	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	// Queue a job
	worker.QueueJob("test-entry-id")

	// Give it a moment
	select {
	case entryID := <-worker.jobChan:
		if entryID != "test-entry-id" {
			t.Errorf("Expected entry ID 'test-entry-id', got '%s'", entryID)
		}
	default:
		t.Error("Expected job to be queued")
	}
}

func TestArchiveWorker_QueueJob_FullQueue(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)

	// Create worker with small channel
	worker := &ArchiveWorker{
		entryRepo: entryRepo,
		dataDir:   "/tmp/test-archives",
		jobChan:   make(chan string, 1), // Small queue
		stopChan:  make(chan struct{}),
		logger:    logging.Default(),
	}

	// Fill the queue
	worker.QueueJob("job-1")
	worker.QueueJob("job-2") // This should be dropped

	// First job should be there
	select {
	case id := <-worker.jobChan:
		if id != "job-1" {
			t.Errorf("Expected job-1, got %s", id)
		}
	default:
		t.Error("Expected at least one job")
	}
}

func TestArchiveWorker_QueueJob_AfterStop_NoPanicNoEnqueue(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	worker.Stop()
	worker.QueueJob("job-after-stop")

	select {
	case <-worker.jobChan:
		t.Fatal("expected no enqueue after stop")
	default:
	}
}

func TestArchiveWorker_Stop_Idempotent(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	worker.Stop()
	worker.Stop() // should not panic
}

func TestFetchURL(t *testing.T) {
	server := createMockServer(200, "<html>Test Page</html>")
	defer server.Close()

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	ctx := context.Background()
	html, status, err := worker.fetchURL(ctx, server.URL)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if status != 200 {
		t.Errorf("Expected status 200, got %d", status)
	}
	if html != "<html>Test Page</html>" {
		t.Errorf("Expected HTML '<html>Test Page</html>', got '%s'", html)
	}
}

func TestFetchURL_404(t *testing.T) {
	server := createMockServer(404, "Not Found")
	defer server.Close()

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	ctx := context.Background()
	html, status, err := worker.fetchURL(ctx, server.URL)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if status != 404 {
		t.Errorf("Expected status 404, got %d", status)
	}
	if html != "Not Found" {
		t.Errorf("Expected 'Not Found', got '%s'", html)
	}
}

func TestFetchURL_InvalidURL(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	ctx := context.Background()
	_, _, err := worker.fetchURL(ctx, "://invalid-url")

	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestGenerateFilename(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/page.html", "example.com_page.html"},
		{"https://example.com/path/to/page?param=value", "example.com_path_to_page_param_value"},
		{"http://localhost:8080/test", "localhost_8080_test"},
	}

	for _, tt := range tests {
		result := worker.generateFilename(tt.url)
		if len(result) == 0 {
			t.Errorf("Expected non-empty filename for %s", tt.url)
		}
		if len(result) > 200 {
			t.Errorf("Expected filename length <= 200, got %d", len(result))
		}
		// Should end with .html
		if len(result) > 5 && result[len(result)-5:] != ".html" {
			t.Errorf("Expected .html extension, got %s", result)
		}
	}
}

func TestSaveToDisk(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "archive-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, tempDir)

	entry := &model.CatalogEntry{
		ID:     "test-save",
		UserID: "test-user",
		URL:    "https://example.com/test",
	}

	html := "<html>Saved Content</html>"
	archivePath, err := worker.saveToDisk(context.Background(), entry, html)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if archivePath == "" {
		t.Error("Expected non-empty archive path")
	}

	// Verify file was created
	_, err = os.Stat(archivePath)
	if os.IsNotExist(err) {
		t.Error("Expected archive file to exist")
	}

	// Verify content
	content, _ := os.ReadFile(archivePath)
	if string(content) != html {
		t.Errorf("Expected content '%s', got '%s'", html, string(content))
	}
}

func TestSaveToDisk_CreateDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "archive-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, tempDir)

	entry := &model.CatalogEntry{
		ID:     "test-dir",
		UserID: "user123",
		URL:    "https://example.com/dir",
	}

	_, err = worker.saveToDisk(context.Background(), entry, "content")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify directory was created
	expectedDir := filepath.Join(tempDir, "archives", "user123")
	_, err = os.Stat(expectedDir)
	if os.IsNotExist(err) {
		t.Error("Expected archive directory to be created")
	}
}

func TestMarkFailed(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	entry := &model.CatalogEntry{
		ID:            "test-fail",
		UserID:        "test-user",
		URL:           "https://example.com/fail",
		ArchiveStatus: model.ArchiveStatusPending,
	}

	// Save entry first
	_ = entryRepo.Create(context.Background(), entry)

	worker.markFailed(context.Background(), entry, "test error")

	// Reload entry
	reloaded, _ := entryRepo.GetByID(context.Background(), "test-fail")
	if reloaded.ArchiveStatus != model.ArchiveStatusFailed {
		t.Errorf("Expected status failed, got %s", reloaded.ArchiveStatus)
	}
}

func TestProcessJob_NotFound(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	// Should not panic
	worker.processJob(context.Background(), "nonexistent-entry")
}

func TestProcessJob_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "archive-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	server := createMockServer(200, "<html>Test Content</html>")
	defer server.Close()

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, tempDir)

	// Create entry
	entry := &model.CatalogEntry{
		ID:            "test-process",
		UserID:        "process-user",
		URL:           server.URL,
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = entryRepo.Create(context.Background(), entry)

	worker.processJob(context.Background(), "test-process")

	// Reload entry
	reloaded, _ := entryRepo.GetByID(context.Background(), "test-process")
	if reloaded.ArchiveStatus != model.ArchiveStatusSuccess {
		t.Errorf("Expected status success, got %s", reloaded.ArchiveStatus)
	}
	if reloaded.ArchivePath == "" {
		t.Error("Expected archive path to be set")
	}
}

func TestProcessJob_FetchError(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	// Create entry with invalid URL
	entry := &model.CatalogEntry{
		ID:            "test-error",
		UserID:        "error-user",
		URL:           "https://invalid-domain-that-does-not-exist-12345.com/",
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = entryRepo.Create(context.Background(), entry)

	// This will fail due to invalid URL, but shouldn't panic
	// Using a context with timeout to prevent long waits
	ctx, cancel := context.WithTimeout(context.Background(), 2)
	defer cancel()

	worker.processJob(ctx, "test-error")
}

func TestProcessJob_HttpError(t *testing.T) {
	server := createMockServer(500, "Server Error")
	defer server.Close()

	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	entry := &model.CatalogEntry{
		ID:            "test-500",
		UserID:        "500-user",
		URL:           server.URL,
		ArchiveStatus: model.ArchiveStatusPending,
	}
	_ = entryRepo.Create(context.Background(), entry)

	worker.processJob(context.Background(), "test-500")

	// Reload entry
	reloaded, _ := entryRepo.GetByID(context.Background(), "test-500")
	if reloaded.ArchiveStatus != model.ArchiveStatusFailed {
		t.Errorf("Expected status failed for 500, got %s", reloaded.ArchiveStatus)
	}
}

func TestStartStop(t *testing.T) {
	db := setupWorkerTestDB(t)
	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, "/tmp/test-archives")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker
	worker.Start(ctx)

	// Give the goroutine time to actually start
	time.Sleep(20 * time.Millisecond)

	// Give it a moment to start
	select {
	case <-worker.stopChan:
		t.Error("Worker should not have stopped yet")
	default:
	}

	// Stop worker - this closes stopChan which triggers ctx cancellation in the worker
	worker.Stop()

	// Wait for worker to process stop signal
	time.Sleep(150 * time.Millisecond)
}

func TestPollPendingEntries(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "archive-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	server := createMockServer(200, "<html>Content</html>")
	defer server.Close()

	// Use unique in-memory DB for this test to avoid pollution from other tests
	// Using a unique file name ensures isolation
	db, err := gorm.Open(sqlite.Open("file:test_poll_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	_ = db.AutoMigrate(&model.CatalogEntry{})

	entryRepo := repository.NewEntryRepository(db)
	worker := NewArchiveWorker(entryRepo, tempDir)

	// Create pending entries
	for i := 0; i < 3; i++ {
		entry := &model.CatalogEntry{
			ID:            "pending-" + string(rune('0'+i)),
			UserID:        "poll-user",
			URL:           server.URL,
			ArchiveStatus: model.ArchiveStatusPending,
		}
		_ = entryRepo.Create(context.Background(), entry)
	}

	// Create already processed entry
	processed := &model.CatalogEntry{
		ID:            "processed-1",
		UserID:        "poll-user",
		URL:           server.URL,
		ArchiveStatus: model.ArchiveStatusSuccess,
		ArchivePath:   "/some/path",
	}
	_ = entryRepo.Create(context.Background(), processed)

	// Poll for pending
	worker.queuePendingEntries(context.Background())

	// Should have queued 3 jobs
	queued := 0
	for {
		select {
		case <-worker.jobChan:
			queued++
		default:
			goto done
		}
	}
done:

	if queued != 3 {
		t.Errorf("Expected 3 jobs queued, got %d", queued)
	}
}
