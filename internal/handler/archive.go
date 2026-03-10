package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/carev01/adhive/internal/config"
	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/repository"
)

// ArchiveOpsHandler exposes archive-centric observability and control endpoints.
type ArchiveOpsHandler struct {
	revisionRepo  *repository.ArchiveRevisionRepository
	assetRepo     *repository.ArchiveAssetRepository
	entryRepo     *repository.EntryRepository
	archiveWorker interface{ QueueJob(entryID string) }
	storageConfig *config.StorageConfig
}

func NewArchiveOpsHandler(revisionRepo *repository.ArchiveRevisionRepository, assetRepo *repository.ArchiveAssetRepository, entryRepo *repository.EntryRepository, archiveWorker interface{ QueueJob(entryID string) }, storageConfig *config.StorageConfig) *ArchiveOpsHandler {
	return &ArchiveOpsHandler{
		revisionRepo:  revisionRepo,
		assetRepo:     assetRepo,
		entryRepo:     entryRepo,
		archiveWorker: archiveWorker,
		storageConfig: storageConfig,
	}
}

type archiveRefreshRequest struct {
	ManualMode bool `json:"manual_mode"`
}

// ListRevisions handles GET /api/v1/entries/:id/archive/revisions
func (h *ArchiveOpsHandler) ListRevisions(c *gin.Context) {
	entryID := c.Param("id")
	userID := c.GetString("user_id")
	if entryID == "" || userID == "" {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "invalid request"))
		return
	}
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}
	revs, err := h.revisionRepo.ListByEntryID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}
	c.JSON(http.StatusOK, gin.H{"revisions": revs})
}

// Refresh handles POST /api/v1/entries/:id/archive/refresh
func (h *ArchiveOpsHandler) Refresh(c *gin.Context) {
	entryID := c.Param("id")
	userID := c.GetString("user_id")
	if entryID == "" || userID == "" {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "invalid request"))
		return
	}
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}
	entry.ArchiveStatus = "pending"
	_ = h.entryRepo.Update(c.Request.Context(), entry)
	manualMode := c.Query("manual_mode") == "true" || c.Query("manual_mode") == "1"
	if !manualMode {
		var req archiveRefreshRequest
		if err := c.ShouldBindJSON(&req); err == nil {
			manualMode = req.ManualMode
		}
	}

	if h.archiveWorker != nil {
		if withOpts, ok := h.archiveWorker.(interface {
			QueueJobWithOptions(entryID string, manualMode bool)
		}); ok {
			withOpts.QueueJobWithOptions(entryID, manualMode)
		} else {
			h.archiveWorker.QueueJob(entryID)
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true, "manual_mode": manualMode})
}

// Metrics handles GET /api/v1/archive/metrics?hours=24
func (h *ArchiveOpsHandler) Metrics(c *gin.Context) {
	hours := 24
	if q := c.Query("hours"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 720 {
			hours = n
		}
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	m, err := h.revisionRepo.Metrics(c.Request.Context(), since)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	partialRate := 0.0
	blockedRate := 0.0
	failedRate := 0.0
	if m.TotalRevisions > 0 {
		partialRate = float64(m.PartialCount) / float64(m.TotalRevisions)
		blockedRate = float64(m.BlockedCount) / float64(m.TotalRevisions)
		failedRate = float64(m.FailedCount) / float64(m.TotalRevisions)
	}
	avgSize := int64(0)
	if m.TotalAssets > 0 {
		avgSize = m.TotalBytes / m.TotalAssets
	}

	c.JSON(http.StatusOK, gin.H{
		"window_hours":        hours,
		"since":               since.UTC().Format(time.RFC3339),
		"metrics":             m,
		"partial_rate":        partialRate,
		"blocked_rate":        blockedRate,
		"failed_rate":         failedRate,
		"average_asset_bytes": avgSize,
	})
}

// DeleteRevision handles DELETE /api/v1/entries/:id/archive/revisions/:revisionId
func (h *ArchiveOpsHandler) DeleteRevision(c *gin.Context) {
	entryID := c.Param("id")
	revisionID := c.Param("revisionId")
	userID := c.GetString("user_id")

	if entryID == "" || revisionID == "" || userID == "" {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "invalid request"))
		return
	}

	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	revision, err := h.revisionRepo.GetByID(c.Request.Context(), revisionID)
	if err != nil || revision == nil || revision.EntryID != entryID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeNotFound, "revision"))
		return
	}

	// Delete assets from DB
	if h.assetRepo != nil {
		if err := h.assetRepo.DeleteByRevisionID(c.Request.Context(), revisionID); err != nil {
			SendError(c, err) // Pass error directly
			return
		}
	}

	// Delete revision folder from disk
	// root_path is stored as "data/archives/{entryID}/rev-XXXX" relative to working directory
	if revision.RootPath != "" {
		absPath := revision.RootPath
		if !filepath.IsAbs(absPath) {
			// root_path is already relative to working directory, use directly
			absPath = filepath.Clean(absPath)
		}
		if err := os.RemoveAll(absPath); err != nil {
			SendError(c, err) // Pass error directly
			return
		}
	}

	// Delete revision record from DB
	if err := h.revisionRepo.DeleteByID(c.Request.Context(), revisionID); err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	// Update entry's current revision if needed
	if entry.ArchiveCurrentRevisionID != nil && *entry.ArchiveCurrentRevisionID == revisionID {
		revs, err := h.revisionRepo.ListByEntryID(c.Request.Context(), entryID)
		if err == nil && len(revs) > 0 {
			entry.ArchiveCurrentRevisionID = &revs[0].ID
		} else {
			entry.ArchiveCurrentRevisionID = nil
		}
		_ = h.entryRepo.Update(c.Request.Context(), entry)
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
