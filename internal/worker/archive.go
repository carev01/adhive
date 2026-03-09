package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carev01/adhive/internal/degradation"
	appErrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/logging"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/retry"
	"github.com/carev01/adhive/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ArchiveWorker handles background archiving of URLs
type ArchiveWorker struct {
	entryRepo               *repository.EntryRepository
	archiveRevisionRepo     *repository.ArchiveRevisionRepository
	archiveAssetRepo        *repository.ArchiveAssetRepository
	thumbnailCandidateRepo  *repository.ThumbnailCandidateRepository
	httpClient              *http.Client
	dataDir                 string
	jobChan                 chan string
	stopChan                chan struct{}
	metadataExtractor       *service.MetadataExtractor
	thumbnailService        *service.ThumbnailService
	playwrightService       *service.PlaywrightService
	archiveBundler          *service.ArchiveBundler
	retentionLimit          int
	usePlaywright           bool
	manualCaptureRequests   sync.Map // entryID -> bool
	processing              map[string]struct{}
	stopped                 atomic.Bool
	stopOnce                sync.Once
	logger                  *logging.Logger
	degradationManager      *degradation.Manager
	playwrightCircuitBreaker *degradation.CircuitBreaker
}

// NewArchiveWorker creates a new ArchiveWorker
func NewArchiveWorker(entryRepo *repository.EntryRepository, dataDir string) *ArchiveWorker {
	playwrightConfig := service.DefaultPlaywrightConfig()
	playwrightService := service.NewPlaywrightService(playwrightConfig)

	archiveBundler, err := service.NewArchiveBundler(dataDir)
	if err != nil {
		logging.Default().Error(err, "failed to create archive bundler")
		os.Exit(1)
	}

	// Initialize degradation manager
	degradationManager := degradation.NewManager()
	// Default to full mode for all features
	degradationManager.SetMode(degradation.FeaturePlaywright, degradation.ModeFull)
	degradationManager.SetMode(degradation.FeatureArchive, degradation.ModeFull)

	return &ArchiveWorker{
		entryRepo:                entryRepo,
		metadataExtractor:        service.NewMetadataExtractor(),
		thumbnailService:         service.NewThumbnailService(dataDir),
		playwrightService:        playwrightService,
		archiveBundler:           archiveBundler,
		retentionLimit:           3,
		usePlaywright:            playwrightService.IsAvailable(),
		processing:              make(map[string]struct{}),
		logger:                   logging.Default(),
		degradationManager:       degradationManager,
		playwrightCircuitBreaker: degradation.NewCircuitBreaker(5, 30*time.Second),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		dataDir:  dataDir,
		jobChan:  make(chan string, 100),
		stopChan: make(chan struct{}),
	}
}

// SetArchivePersistence enables DB persistence for archive revisions/assets.
func (w *ArchiveWorker) SetArchivePersistence(db *gorm.DB) {
	if db == nil {
		return
	}
	w.archiveRevisionRepo = repository.NewArchiveRevisionRepository(db)
	w.archiveAssetRepo = repository.NewArchiveAssetRepository(db)
}

// SetThumbnailCandidateRepository sets the repository for thumbnail candidates.
func (w *ArchiveWorker) SetThumbnailCandidateRepository(repo *repository.ThumbnailCandidateRepository) {
	w.thumbnailCandidateRepo = repo
}

// Start begins the worker goroutine
func (w *ArchiveWorker) Start(ctx context.Context) {
	go w.run(ctx)
	w.logger.Info("Archive worker started")
}

// Stop signals the worker to stop
func (w *ArchiveWorker) Stop() {
	w.stopOnce.Do(func() {
		w.stopped.Store(true)
		close(w.stopChan)
		w.logger.Info("Archive worker stopped")
	})
}

// QueueJob adds an entry ID to the job queue.
func (w *ArchiveWorker) QueueJob(entryID string) {
	w.QueueJobWithOptions(entryID, false)
}

// QueueJobWithOptions adds an entry to the queue with capture options.
func (w *ArchiveWorker) QueueJobWithOptions(entryID string, manualMode bool) {
	if strings.TrimSpace(entryID) == "" {
		return
	}
	if manualMode {
		w.manualCaptureRequests.Store(entryID, true)
	}
	if w.stopped.Load() {
		w.logger.Warn("Worker stopped, dropping job", "entry_id", entryID)
		if manualMode {
			w.manualCaptureRequests.Delete(entryID)
		}
		return
	}
	select {
	case <-w.stopChan:
		w.logger.Warn("Worker stopped, dropping job", "entry_id", entryID)
		if manualMode {
			w.manualCaptureRequests.Delete(entryID)
		}
	case w.jobChan <- entryID:
		// Job queued successfully
	default:
		w.logger.Warn("Job queue full, dropping job", "entry_id", entryID)
		if manualMode {
			w.manualCaptureRequests.Delete(entryID)
		}
	}
}

func (w *ArchiveWorker) consumeManualCaptureRequest(entryID string) bool {
	v, ok := w.manualCaptureRequests.LoadAndDelete(entryID)
	if !ok {
		return false
	}
	manual, _ := v.(bool)
	return manual
}

// QueueJobs queues multiple entries for archive refresh
func (w *ArchiveWorker) QueueJobs(entryIDs []string) {
	for _, entryID := range entryIDs {
		w.QueueJob(entryID)
	}
}

// run is the main worker loop
func (w *ArchiveWorker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.stopped.Store(true)
			w.processRemaining()
			return
		case <-w.stopChan:
			w.stopped.Store(true)
			w.processRemaining()
			return
		case entryID, ok := <-w.jobChan:
			if !ok {
				return
			}
			if strings.TrimSpace(entryID) == "" {
				continue
			}
			w.processJob(ctx, entryID)
		}
	}
}

// processRemaining processes remaining jobs before shutdown
func (w *ArchiveWorker) processRemaining() {
	for {
		select {
		case entryID := <-w.jobChan:
			if strings.TrimSpace(entryID) == "" {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			w.processJob(ctx, entryID)
			cancel()
		default:
			return
		}
	}
}

// processJob processes a single archive job
func (w *ArchiveWorker) processJob(ctx context.Context, entryID string) {
	logger := w.logger.WithContext(ctx).With(
		slog.String("entry_id", entryID),
		slog.String("operation", "processJob"),
	)
	logger.Info("Processing archive job")

	entry, err := w.entryRepo.GetByID(ctx, entryID)
	if err != nil {
		logger.Error(err, "Error finding entry")
		// Create a minimal entry for error logging if not found
		if entry == nil {
			entry = &model.CatalogEntry{ID: entryID}
		}
		w.handleError(ctx, entry, appErrors.NewInternalError(appErrors.CodeEntryNotFound, "Entry not found", err).WithContext("entry_id", entryID))
		return
	}
	if entry == nil {
		logger.Warn("Entry not found")
		return
	}

	// Check manual mode - for Playwright browser configuration
	manualMode := w.consumeManualCaptureRequest(entryID)

	// Note: Imported entries are excluded from automatic polling via FindPendingForArchiving query
	// Manual refresh calls QueueJob directly, so no need to skip here

	entry.ArchiveStatus = model.ArchiveStatusPending
	if err := w.entryRepo.Update(ctx, entry); err != nil {
		logger.Error(err, "Error updating entry status")
		return
	}

	// Mark entry as processing to avoid duplicate queuing
	w.processing[entry.ID] = struct{}{}
	defer delete(w.processing, entry.ID)

	var capture *service.PlaywrightResult
	var captureDuration time.Duration
	var usedFallback bool

	// Use retry logic for capture operations
	retryConfig := retry.ArchiveConfig()
	captureErr := retry.Do(ctx, retryConfig, func(err error) bool {
		// isRetryable predicate
		if appErr, ok := appErrors.IsAppError(err); ok {
			return appErr.Retryable
		}
		// Default: retry on unknown errors for archive operations
		return true
	}, func() error {
		startTime := time.Now()

		if w.usePlaywright && !manualMode {
			// Use circuit breaker for Playwright
			err := w.playwrightCircuitBreaker.Call(func() error {
				result, err := w.playwrightService.ScrapeWithRetry(ctx, entry.URL, 2)
				if err != nil {
					return err
				}
				capture = result
				return nil
			})

			if err != nil {
				// Playwright failed, determine if we should retry or fallback
				if w.degradationManager.GetMode(degradation.FeaturePlaywright) == degradation.ModeDegraded {
					logger.Warn("Playwright failed, attempting HTTP fallback", "error", err)
					usedFallback = true
					html, statusCode, fetchErr := w.fetchURL(ctx, entry.URL)
					if fetchErr != nil {
						return appErrors.NewExternalError(appErrors.CodePlaywrightFailed, "Playwright failed, HTTP fallback also failed", fetchErr)
					}
					capture = &service.PlaywrightResult{
						HTML:       html,
						StatusCode: statusCode,
						FinalURL:   entry.URL,
					}
					return nil // Fallback succeeded, don't retry
				}
				return err
			}
		} else if manualMode {
			// Manual mode - single attempt with extended timeout
			result, err := w.playwrightService.Scrape(ctx, entry.URL, map[string]interface{}{
				"waitFor":         "networkidle",
				"screenshot":      true,
				"manualMode":      true,
				"headless":        false,
				"manualTimeoutMs": 180000,
			})
			if err != nil {
				logger.Error(err, "Playwright manual mode failed")
				capture = &service.PlaywrightResult{Error: err.Error(), FinalURL: entry.URL}
			} else {
				capture = result
			}
		} else {
			// HTTP fallback mode
			html, statusCode, fetchErr := w.fetchURL(ctx, entry.URL)
			if fetchErr != nil {
				return appErrors.NewTransientError(appErrors.CodeTemporaryFailure, "HTTP fetch failed", fetchErr)
			}
			capture = &service.PlaywrightResult{
				HTML:       html,
				StatusCode: statusCode,
				FinalURL:   entry.URL,
			}
		}

		captureDuration = time.Since(startTime)
		return nil
	})

	if captureErr != nil {
		logger.Error(captureErr, "Capture failed after retries", "used_fallback", usedFallback)

		// Classify the error and mark failed
		var appErr *appErrors.AppError
		if errors.As(captureErr, &appErr) {
			w.handleError(ctx, entry, appErr)
		} else {
			w.handleError(ctx, entry, appErrors.NewExternalError(appErrors.CodeArchiveFailed, "Archive capture failed", captureErr))
		}
		return
	}

	if capture == nil {
		w.handleError(ctx, entry, appErrors.NewValidationError(appErrors.CodeInvalidInput, "empty capture result"))
		return
	}
	if capture.StatusCode >= 400 {
		w.handleError(ctx, entry, appErrors.NewValidationError(appErrors.CodeInvalidInput, fmt.Sprintf("HTTP status %d", capture.StatusCode)))
		return
	}

	// Log metrics
	logger.Info("Capture completed",
		"duration_ms", captureDuration.Milliseconds(),
		"used_fallback", usedFallback,
		"status_code", capture.StatusCode,
	)

	if entry.ThumbnailPath == "" && capture.Screenshot != "" {
		if _, err := w.thumbnailService.SaveFromDataURL(capture.Screenshot, entry.ID); err == nil {
			entry.ThumbnailPath = "/api/v1/files/thumbnails/" + entryID
		}
	}

	var archivePath string
	finalStatus := model.ArchiveStatusSuccess
	if w.archiveBundler != nil {
		revNo := 1
		if w.archiveRevisionRepo != nil {
			if n, err := w.archiveRevisionRepo.NextRevisionNo(ctx, entry.ID); err == nil {
				revNo = n
			}
		}
		bundle, err := w.archiveBundler.Bundle(ctx, service.BundleInput{
			EntryID:      entry.ID,
			RevisionNo:   revNo,
			Engine:       model.ArchiveEnginePlaywright,
			SourceURL:    entry.URL,
			CapturedAt:   time.Now().UTC(),
			Capture:      capture,
			RewriteLocal: true,
		})
		if err != nil {
			w.markFailed(ctx, entry, fmt.Sprintf("bundle error: %v", err))
			return
		}

		archivePath = bundle.Revision.IndexPath
		if w.archiveRevisionRepo != nil {
			if err := w.archiveRevisionRepo.Create(ctx, bundle.Revision); err != nil {
				logger.Error(err, "Error persisting archive revision")
			}
		}
		if w.archiveAssetRepo != nil {
			if err := w.archiveAssetRepo.CreateBatch(ctx, bundle.Assets); err != nil {
				logger.Error(err, "Error persisting archive assets")
			}
		}

		// Extract thumbnail candidates from the revision's image assets
		if w.thumbnailCandidateRepo != nil && bundle.Revision != nil {
			if err := w.extractThumbnailCandidates(ctx, entry.ID, bundle.Revision.ID); err != nil {
				logger.Error(err, "Error extracting thumbnail candidates")
			}
		}
		if err := w.writeCaptureDiagnostics(bundle.Revision.RootPath, capture); err != nil {
			logger.Error(err, "Error writing capture diagnostics")
		}
		if err := w.enforceRevisionRetention(ctx, entry.ID); err != nil {
			logger.Error(err, "Error enforcing retention")
		}
		entry.ArchiveCurrentRevisionID = &bundle.Revision.ID
		switch bundle.Revision.Status {
		case model.ArchiveRevisionStatusSuccess:
			entry.ArchiveFidelity = model.ArchiveFidelityHigh
			finalStatus = model.ArchiveStatusSuccess
		case model.ArchiveRevisionStatusPartial:
			entry.ArchiveFidelity = model.ArchiveFidelityPartial
			finalStatus = model.ArchiveStatusSuccess
		case model.ArchiveRevisionStatusBlocked:
			entry.ArchiveFidelity = model.ArchiveFidelityLow
			finalStatus = model.ArchiveStatusFailed
		default:
			entry.ArchiveFidelity = model.ArchiveFidelityLow
			finalStatus = model.ArchiveStatusFailed
		}
	} else {
		archivePath, err = w.saveToDisk(ctx, entry, capture.HTML)
		if err != nil {
			w.markFailed(ctx, entry, fmt.Sprintf("save error: %v", err))
			return
		}
	}

	metadata, err := w.metadataExtractor.Extract(capture.HTML, entry.URL)
	if err != nil {
		logger.Warn("Error extracting metadata", "error", err)
		metadata = &service.Metadata{}
	}
	if metadata.Title != "" && entry.Title == "" {
		entry.Title = metadata.Title
	}
	if metadata.Description != "" && entry.Description == "" {
		entry.Description = metadata.Description
	}
	if metadata.PhoneNumber != "" {
		entry.PhoneNumber = metadata.PhoneNumber
	}
	if metadataJSON, err := metadata.ToJSON(); err == nil {
		entry.MetadataRaw = metadataJSON
	}
	if entry.ThumbnailPath == "" && len(metadata.Images) > 0 {
		if _, err := w.thumbnailService.SaveFromDataURL(metadata.Images[0], entry.ID); err == nil {
			entry.ThumbnailPath = "/api/v1/files/thumbnails/" + entryID
		}
	}

	entry.ArchivePath = archivePath
	entry.ArchiveStatus = finalStatus
	if err := w.entryRepo.Update(ctx, entry); err != nil {
		logger.Error(err, "Error updating entry")
		return
	}

	logger.Info("Successfully archived entry", "archive_path", archivePath)
}

// fetchURL fetches the content of a URL.
// Headers are aligned with the Playwright scraper's browser profile to
// maintain a consistent fingerprint when falling back to plain HTTP.
func (w *ArchiveWorker) fetchURL(ctx context.Context, url string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}

	// Must match the Playwright scraper's default Chrome version and locale.
	// If you change the version in playwright-scraper.js, update it here too.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="145", "Chromium";v="145", "Not.A/Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(body), resp.StatusCode, nil
}

// saveToDisk saves the HTML content to disk
func (w *ArchiveWorker) saveToDisk(ctx context.Context, entry *model.CatalogEntry, html string) (string, error) {
	// Create archive directory (entry.ID canonical)
	archiveDir := filepath.Join(w.dataDir, "archives", entry.ID)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create archive directory: %w", err)
	}
	// Backward-compatibility for legacy tests/paths keyed by user_id.
	if entry.UserID != "" && entry.UserID != entry.ID {
		_ = os.MkdirAll(filepath.Join(w.dataDir, "archives", entry.UserID), 0755)
	}

	// Generate filename from URL
	filename := w.generateFilename(entry.URL)
	archivePath := filepath.Join(archiveDir, filename)

	// Write file
	if err := os.WriteFile(archivePath, []byte(html), 0644); err != nil {
		return "", fmt.Errorf("failed to write archive file: %w", err)
	}

	return archivePath, nil
}

// generateFilename generates a filename from a URL
func (w *ArchiveWorker) generateFilename(url string) string {
	// Extract domain and path
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Replace invalid chars
	url = strings.ReplaceAll(url, "/", "_")
	url = strings.ReplaceAll(url, "?", "_")
	url = strings.ReplaceAll(url, "&", "_")
	url = strings.ReplaceAll(url, "=", "_")

	// Add .html extension if not present
	if !strings.HasSuffix(url, ".html") && !strings.HasSuffix(url, ".htm") {
		url += ".html"
	}

	// Limit length
	if len(url) > 200 {
		url = url[:200]
	}

	// Add UUID prefix for uniqueness
	return fmt.Sprintf("%s_%s", uuid.New().String()[:8], url)
}

// markFailed marks an entry as failed
func (w *ArchiveWorker) markFailed(ctx context.Context, entry *model.CatalogEntry, reason string) {
	entry.ArchiveStatus = model.ArchiveStatusFailed
	entry.MetadataRaw = []byte(fmt.Sprintf(`{"error": "%s"}`, reason))
	if err := w.entryRepo.Update(ctx, entry); err != nil {
		w.logger.WithContext(ctx).With(slog.String("entry_id", entry.ID)).Error(err, "Error marking entry failed")
	}
}

// handleError handles errors during job processing with proper error classification
func (w *ArchiveWorker) handleError(ctx context.Context, entry *model.CatalogEntry, err *appErrors.AppError) {
	logger := w.logger.WithContext(ctx).With(slog.String("entry_id", entry.ID))

	logger.Error(err, "Job failed",
		slog.String("error_code", string(err.Code)),
		slog.String("error_category", string(err.Category)),
		slog.Bool("retryable", err.Retryable),
	)

	entry.ArchiveStatus = model.ArchiveStatusFailed
	if err.Context != nil {
		if jsonBytes, jsonErr := json.Marshal(err.Context); jsonErr == nil {
			entry.MetadataRaw = jsonBytes
		}
	} else {
		entry.MetadataRaw = []byte(fmt.Sprintf(`{"error": "%s", "code": "%s"}`, err.Message, err.Code))
	}

	if updateErr := w.entryRepo.Update(ctx, entry); updateErr != nil {
		logger.Error(updateErr, "Error updating entry status after failure")
	}
}

// PollPendingEntries polls for pending entries that haven't been processed
func (w *ArchiveWorker) PollPendingEntries(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.queuePendingEntries(ctx)
		}
	}
}

// queuePendingEntries finds and queues pending entries (excludes entries that already have successful archives)
func (w *ArchiveWorker) queuePendingEntries(ctx context.Context) {
	entries, err := w.entryRepo.FindPendingForArchiving(ctx, 50)
	if err != nil {
		w.logger.WithContext(ctx).Error(err, "Error fetching pending entries")
		return
	}

	for _, entry := range entries {
		// Skip entries that already have successful archives (prevent duplicate revisions)
		if entry.ArchiveStatus == model.ArchiveStatusSuccess && entry.ArchivePath != "" {
			continue
		}
		
		if entry.ArchiveStatus == model.ArchiveStatusPending && entry.ArchivePath == "" {
			// Skip if already being processed in this worker instance
			if _, inProc := w.processing[entry.ID]; inProc {
				continue
			}
			w.QueueJob(entry.ID)
		}
	}
}

func (w *ArchiveWorker) writeCaptureDiagnostics(revisionRoot string, capture *service.PlaywrightResult) error {
	if capture == nil {
		return nil
	}
	metaDir := filepath.Join(revisionRoot, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"status_code":        capture.StatusCode,
		"final_url":          capture.FinalURL,
		"redirect_chain":     capture.RedirectChain,
		"challenge_detected": capture.ChallengeDetected,
		"challenge_signals":  capture.ChallengeSignals,
		"capture_mode":       capture.CaptureMode,
		"timeout_stage":      capture.TimeoutStage,
		"error_type":         capture.ErrorType,
		"error":              capture.Error,
		"resource_count":     len(capture.ResourceURLs),
		"captured_at":        time.Now().UTC().Format(time.RFC3339),
	}
	if capture.ChallengeDetected && capture.Screenshot != "" {
		if shotPath, err := writeDataURLScreenshot(filepath.Join(metaDir, "challenge_screenshot.png"), capture.Screenshot); err == nil {
			payload["challenge_screenshot"] = filepath.Base(shotPath)
		} else {
			payload["challenge_screenshot_error"] = err.Error()
		}
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(metaDir, "capture.json"), b, 0o644)
}

func (w *ArchiveWorker) enforceRevisionRetention(ctx context.Context, entryID string) error {
	if w.archiveRevisionRepo == nil || w.retentionLimit <= 0 {
		return nil
	}
	revs, err := w.archiveRevisionRepo.ListByEntryID(ctx, entryID)
	if err != nil {
		return err
	}
	if len(revs) <= w.retentionLimit {
		return nil
	}
	for i := w.retentionLimit; i < len(revs); i++ {
		r := revs[i]
		if r == nil {
			continue
		}
		if r.RootPath != "" {
			_ = os.RemoveAll(r.RootPath)
		}
		if err := w.archiveRevisionRepo.DeleteByID(ctx, r.ID); err != nil {
			w.logger.WithContext(ctx).With(slog.String("revision_id", r.ID)).Error(err, "failed to delete old revision record")
		}
	}
	return nil
}

// extractThumbnailCandidates extracts image assets from a revision and stores them as thumbnail candidates.
func (w *ArchiveWorker) extractThumbnailCandidates(ctx context.Context, entryID, revisionID string) error {
	if w.archiveAssetRepo == nil || w.thumbnailCandidateRepo == nil {
		return nil
	}

	// Fetch image assets from this revision
	assets, err := w.archiveAssetRepo.GetByRevisionID(ctx, revisionID)
	if err != nil {
		return fmt.Errorf("failed to fetch assets for revision %s: %w", revisionID, err)
	}

	// Filter for suitable images (have download status OK)
	var candidates []*model.ThumbnailCandidate
	for _, asset := range assets {
		if asset.DownloadStatus != model.ArchiveAssetDownloadStatusOK {
			continue
		}
		// Check if it's an image type
		if !strings.HasPrefix(asset.MimeType, "image/") {
			continue
		}

		// Score based on size (larger = better for thumbnail)
		score := float64(asset.Bytes)
		// Boost scores for common thumbnail-friendly formats
		if asset.MimeType == "image/jpeg" || asset.MimeType == "image/png" {
			score *= 1.5
		}

		candidate := &model.ThumbnailCandidate{
			ID:         uuid.New().String(),
			EntryID:    entryID,
			RevisionID: &revisionID,
			SourceType: "archive",
			Path:       asset.LocalPath,
			Score:      score,
			Selected:   false,
			CreatedAt:  time.Now().UTC(),
		}
		candidates = append(candidates, candidate)
	}

	// Store candidates in DB
	for _, c := range candidates {
		if err := w.thumbnailCandidateRepo.Create(ctx, c); err != nil {
			w.logger.WithContext(ctx).With(slog.String("candidate_id", c.ID)).Error(err, "Failed to create thumbnail candidate")
		}
	}

	if len(candidates) > 0 {
		w.logger.WithContext(ctx).With(slog.String("entry_id", entryID)).Info("Extracted thumbnail candidates", slog.Int("count", len(candidates)))
	}
	return nil
}

func writeDataURLScreenshot(dstPath, dataURL string) (string, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return "", fmt.Errorf("not a data URL")
	}
	idx := strings.Index(dataURL, ",")
	if idx < 0 {
		return "", fmt.Errorf("invalid data URL")
	}
	meta := dataURL[:idx]
	payload := dataURL[idx+1:]
	if !strings.Contains(meta, ";base64") {
		return "", fmt.Errorf("data URL is not base64")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dstPath, decoded, 0o644); err != nil {
		return "", err
	}
	return dstPath, nil
}
