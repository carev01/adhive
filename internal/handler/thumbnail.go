package handler

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ThumbnailHandler handles thumbnail candidate APIs.
type ThumbnailHandler struct {
	entryRepo              *repository.EntryRepository
	archiveAssetRepo       *repository.ArchiveAssetRepository
	archiveRevisionRepo    *repository.ArchiveRevisionRepository
	thumbnailCandidateRepo *repository.ThumbnailCandidateRepository
	thumbnailService       *service.ThumbnailService
	dataDir                string
}

func NewThumbnailHandler(
	entryRepo *repository.EntryRepository,
	archiveAssetRepo *repository.ArchiveAssetRepository,
	archiveRevisionRepo *repository.ArchiveRevisionRepository,
	thumbnailCandidateRepo *repository.ThumbnailCandidateRepository,
	thumbnailService *service.ThumbnailService,
	dataDir string,
) *ThumbnailHandler {
	return &ThumbnailHandler{
		entryRepo:              entryRepo,
		archiveAssetRepo:       archiveAssetRepo,
		archiveRevisionRepo:    archiveRevisionRepo,
		thumbnailCandidateRepo: thumbnailCandidateRepo,
		thumbnailService:       thumbnailService,
		dataDir:                dataDir,
	}
}

type thumbnailCandidateResponse struct {
	ID         string  `json:"id"`
	EntryID    string  `json:"entry_id"`
	RevisionID *string `json:"revision_id,omitempty"`
	SourceType string  `json:"source_type"`
	Path       string  `json:"path"`
	Score      float64 `json:"score"`
	Selected   bool    `json:"selected"`
}

type thumbnailCandidatesListResponse struct {
	Candidates []thumbnailCandidateResponse `json:"candidates"`
}

type selectThumbnailRequest struct {
	CandidateID string `json:"candidate_id" binding:"required"`
}

func (h *ThumbnailHandler) ListCandidates(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	entryID := c.Param("entryID")
	if entryID == "" {
		entryID = c.Param("id")
	}
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch entry"})
		return
	}
	if entry == nil || entry.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	candidates, err := h.thumbnailCandidateRepo.ListByEntryID(c.Request.Context(), entryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list candidates"})
		return
	}

	if len(candidates) == 0 {
		candidates, err = h.generateCandidates(c, entry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	resp := make([]thumbnailCandidateResponse, 0, len(candidates))
	for _, cand := range candidates {
		resp = append(resp, thumbnailCandidateResponse{
			ID:         cand.ID,
			EntryID:    cand.EntryID,
			RevisionID: cand.RevisionID,
			SourceType: string(cand.SourceType),
			Path:       toPublicThumbnailPath(cand.Path, h.dataDir),
			Score:      cand.Score,
			Selected:   cand.Selected,
		})
	}

	c.JSON(http.StatusOK, thumbnailCandidatesListResponse{Candidates: resp})
}

func (h *ThumbnailHandler) SelectCandidate(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	entryID := c.Param("entryID")
	if entryID == "" {
		entryID = c.Param("id")
	}
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch entry"})
		return
	}
	if entry == nil || entry.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	var req selectThumbnailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cand, err := h.thumbnailCandidateRepo.GetByID(c.Request.Context(), req.CandidateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "candidate not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch candidate"})
		return
	}
	if cand.EntryID != entryID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "candidate does not belong to entry"})
		return
	}

	if err := h.thumbnailCandidateRepo.Select(c.Request.Context(), entryID, cand.ID); err != nil {
		log.Printf("SelectCandidate: failed to select candidate %s for entry %s: %v", cand.ID, entryID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to select candidate"})
		return
	}

	// Materialize the selected candidate to the main thumbnail location
	thumbnailDir := filepath.Join(h.dataDir, "thumbnails", entryID)
	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		log.Printf("SelectCandidate: failed to create thumbnail dir for entry %s: %v", entryID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create thumbnail directory"})
		return
	}

	// Resolve the source path for the candidate
	sourcePath := cand.Path
	if !filepath.IsAbs(sourcePath) {
		// Relative path - needs resolution
		if cand.RevisionID != nil && *cand.RevisionID != "" {
			// Archive asset with revision - look up the revision to get its root_path
			revision, err := h.archiveRevisionRepo.GetByID(c.Request.Context(), *cand.RevisionID)
			if err != nil || revision == nil {
				log.Printf("SelectCandidate: revision %s not found for candidate %s", *cand.RevisionID, cand.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "revision not found"})
				return
			}
			// revision.RootPath is like "data/archives/{entryID}/rev-0002"
			if revision.RootPath != "" {
				sourcePath = filepath.Join(revision.RootPath, filepath.FromSlash(cand.Path))
			} else {
				// Fallback to old behavior
				sourcePath = filepath.Join(h.dataDir, "archives", entryID, *cand.RevisionID, filepath.FromSlash(cand.Path))
			}
		} else {
			// Try candidates directory
			sourcePath = filepath.Join(h.dataDir, "thumbnails", entryID, "candidates", cand.Path)
		}
	}

	// Verify the file exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		log.Printf("SelectCandidate: candidate file not found at %s for entry %s", sourcePath, entryID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "candidate file not found"})
		return
	}

	// Copy the selected candidate to the main thumbnail location
	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Printf("SelectCandidate: failed to read candidate file %s for entry %s: %v", cand.Path, entryID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read selected thumbnail"})
		return
	}

	destPath := filepath.Join(thumbnailDir, entryID+".webp")
	if err := os.WriteFile(destPath, sourceData, 0644); err != nil {
		log.Printf("SelectCandidate: failed to write thumbnail for entry %s: %v", entryID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save selected thumbnail"})
		return
	}

	entry.ThumbnailPath = "/api/v1/files/thumbnails/" + entryID
	entry.ThumbnailSource = model.ThumbnailSourceUserSelected
	if err := h.entryRepo.Update(c.Request.Context(), entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update entry"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"thumbnail_path":        entry.ThumbnailPath,
		"thumbnail_source":      entry.ThumbnailSource,
		"selected_candidate_id": cand.ID,
	})
}

func (h *ThumbnailHandler) generateCandidates(c *gin.Context, entry *model.CatalogEntry) ([]*model.ThumbnailCandidate, error) {
	assets, err := h.archiveAssetRepo.ListImageAssetsByEntry(c.Request.Context(), entry.ID)
	if err != nil {
		return nil, err
	}

	inputs := make([]service.CandidateInput, 0, len(assets)+2)
	for _, asset := range assets {
		absPath := asset.LocalPath
		if !filepath.IsAbs(absPath) {
			// Use RootPath from the revision (populated via JOIN)
			if asset.RootPath != "" {
				absPath = filepath.Join(asset.RootPath, filepath.FromSlash(asset.LocalPath))
			} else {
				// Fallback to old behavior
				absPath = filepath.Join(h.dataDir, "archives", entry.ID, filepath.FromSlash(asset.LocalPath))
			}
		}
		if st, statErr := os.Stat(absPath); statErr != nil || st.IsDir() {
			continue
		}

		revID := asset.RevisionID
		inputs = append(inputs, service.CandidateInput{
			SourcePath: absPath,
			SourceURL:  asset.SourceURL,
			SourceType: model.ThumbnailCandidateSourceLocalAsset,
			RevisionID: &revID,
		})
	}

	if entry.ThumbnailPath != "" {
		screenshotPath := filepath.Join(h.dataDir, "thumbnails", entry.ID+".webp")
		if st, statErr := os.Stat(screenshotPath); statErr == nil && !st.IsDir() {
			inputs = append(inputs, service.CandidateInput{
				SourcePath: screenshotPath,
				SourceType: model.ThumbnailCandidateSourceScreenshot,
			})
		}
	}

	results, err := h.thumbnailService.BuildCandidatesFromLocalAssets(entry.ID, inputs)
	if err != nil {
		return nil, err
	}

	created := make([]*model.ThumbnailCandidate, 0, len(results))
	for i, result := range results {
		cand := &model.ThumbnailCandidate{
			ID:         uuid.New().String(),
			EntryID:    entry.ID,
			RevisionID: result.RevisionID,
			SourceType: result.SourceType,
			Path:       result.Path,
			Score:      result.Score,
			Selected:   i == 0,
		}
		if err := h.thumbnailCandidateRepo.Create(c.Request.Context(), cand); err != nil {
			return nil, err
		}
		created = append(created, cand)
	}

	if len(created) > 0 {
		// Set default selected thumbnail in entry as first candidate.
		entry.ThumbnailPath = "/api/v1/files/thumbnails/" + entry.ID
		entry.ThumbnailSource = model.ThumbnailSourceAuto
		_ = h.entryRepo.Update(c.Request.Context(), entry)
	}

	return created, nil
}

func toPublicThumbnailPath(absPath, dataDir string) string {
	clean := filepath.Clean(absPath)
	// Convert dataDir to absolute for consistent comparison
	dataDirAbs, err := filepath.Abs(dataDir)
	if err != nil {
		dataDirAbs = dataDir
	}
	root := filepath.Clean(filepath.Join(dataDirAbs, "thumbnails"))
	if strings.HasPrefix(clean, root) {
		rel := strings.TrimPrefix(clean, root)
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
		return "/api/v1/files/thumbnails/raw/" + rel
	}
	return absPath
}
