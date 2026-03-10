package handler

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/carev01/adhive/internal/config"
	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
)

// EntryHandler handles catalog entry HTTP requests
type EntryHandler struct {
	entryRepo     *repository.EntryRepository
	tagRepo       *repository.TagRepository
	storageConfig *config.StorageConfig
	archiveWorker interface {
		QueueJob(entryID string)
		QueueJobs(entryIDs []string)
	}
}

// NewEntryHandler creates a new EntryHandler
func NewEntryHandler(entryRepo *repository.EntryRepository, tagRepo *repository.TagRepository) *EntryHandler {
	return &EntryHandler{
		entryRepo: entryRepo,
		tagRepo:   tagRepo,
	}
}

// NewEntryHandlerWithStorage creates an EntryHandler with storage config for cascade delete
func NewEntryHandlerWithStorage(entryRepo *repository.EntryRepository, tagRepo *repository.TagRepository, storageConfig *config.StorageConfig) *EntryHandler {
	return &EntryHandler{
		entryRepo:     entryRepo,
		tagRepo:       tagRepo,
		storageConfig: storageConfig,
	}
}

// SetArchiveWorker sets the archive worker for auto-archiving
func (h *EntryHandler) SetArchiveWorker(w interface {
	QueueJob(entryID string)
	QueueJobs(entryIDs []string)
}) {
	h.archiveWorker = w
}

// Request/Response DTOs
type CreateEntryRequest struct {
	URL         string `json:"url" binding:"required,url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PhoneNumber string `json:"phone_number"`
	Location    string `json:"location"`
}

type UpdateEntryRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	PhoneNumber   *string `json:"phone_number"`
	Location      *string `json:"location"`
	ThumbnailPath *string `json:"thumbnail_path"`
	ArchivePath   *string `json:"archive_path"`
	ArchiveStatus *string `json:"archive_status"`
}

type EntryResponse struct {
	ID                       string    `json:"id"`
	UserID                   string    `json:"user_id"`
	URL                      string    `json:"url"`
	Title                    string    `json:"title,omitempty"`
	Description              string    `json:"description,omitempty"`
	PhoneNumber              string    `json:"phone_number,omitempty"`
	Location                 string    `json:"location,omitempty"`
	ThumbnailPath            string    `json:"thumbnail_path,omitempty"`
	ArchivePath              string    `json:"archive_path,omitempty"`
	ArchiveStatus            string    `json:"archive_status"`
	ArchiveFidelity          string    `json:"archive_fidelity,omitempty"`
	ArchiveCurrentRevisionID string    `json:"archive_current_revision_id,omitempty"`
	ThumbnailSource          string    `json:"thumbnail_source,omitempty"`
	Tags                     []TagInfo `json:"tags,omitempty"`
	CreatedAt                string    `json:"created_at"`
	UpdatedAt                string    `json:"updated_at"`
}

type TagInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type EntryListResponse struct {
	Entries []*EntryResponse `json:"entries"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
}

func entryToResponse(entry *model.CatalogEntry, tags []TagInfo) *EntryResponse {
	if entry == nil {
		return nil
	}
	revID := ""
	if entry.ArchiveCurrentRevisionID != nil {
		revID = *entry.ArchiveCurrentRevisionID
	}
	return &EntryResponse{
		ID:                       entry.ID,
		UserID:                   entry.UserID,
		URL:                      entry.URL,
		Title:                    entry.Title,
		Description:              entry.Description,
		PhoneNumber:              entry.PhoneNumber,
		Location:                 entry.Location,
		ThumbnailPath:            entry.ThumbnailPath,
		ArchivePath:              entry.ArchivePath,
		ArchiveStatus:            string(entry.ArchiveStatus),
		ArchiveFidelity:          string(entry.ArchiveFidelity),
		ArchiveCurrentRevisionID: revID,
		ThumbnailSource:          string(entry.ThumbnailSource),
		Tags:                     tags,
		CreatedAt:                entry.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:                entry.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// List handles GET /api/v1/entries
func (h *EntryHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	tagID := c.Query("tag")
	status := c.Query("status")
	excludeTried := c.Query("exclude_tried") == "true"
	search := c.Query("search")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	source := c.Query("source")
	location := c.Query("location")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	hasInteraction := c.Query("has_interaction") == "true"
	minScore, _ := strconv.Atoi(c.Query("min_score"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	filter := &model.EntryFilter{
		Page:           page,
		Limit:          limit,
		TagID:          tagID,
		ExcludeTried:   excludeTried,
		Search:         search,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		Source:         source,
		Location:       location,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		HasInteraction: hasInteraction,
		MinScore:       minScore,
	}
	if status != "" {
		filter.Status = model.ArchiveStatus(status)
	}

	var result *model.EntryListResult
	var err error

	ctx := c.Request.Context()

	if search != "" {
		result, err = h.entryRepo.Search(ctx, userID, search, filter)
	} else {
		result, err = h.entryRepo.GetByUserID(ctx, userID, filter)
	}

	if err != nil {
		// Pass error directly - SendError handles AppError types
		SendError(c, err)
		return
	}

	entries := make([]*EntryResponse, len(result.Entries))
	for i, e := range result.Entries {
		// Fetch tags for each entry (could be optimized with a single query)
		tags, _ := h.tagRepo.GetEntryTags(e.ID)
		tagInfos := make([]TagInfo, len(tags))
		for j, t := range tags {
			tagInfos[j] = TagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
		}
		entries[i] = entryToResponse(e, tagInfos)
	}

	c.JSON(http.StatusOK, EntryListResponse{
		Entries: entries,
		Total:   result.Total,
		Page:    result.Page,
		Limit:   result.Limit,
	})
}

// Sources handles GET /api/v1/entries/sources
func (h *EntryHandler) Sources(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	sources, err := h.entryRepo.GetUserSources(c.Request.Context(), userID)
	if err != nil {
		// Pass error directly - SendError handles AppError types
		SendError(c, err)
		return
	}

	c.JSON(http.StatusOK, map[string][]string{"sources": sources})
}

// Locations handles GET /api/v1/entries/locations
func (h *EntryHandler) Locations(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	locations, err := h.entryRepo.GetUserLocations(c.Request.Context(), userID)
	if err != nil {
		// Pass error directly - SendError handles AppError types
		SendError(c, err)
		return
	}

	c.JSON(http.StatusOK, map[string][]string{"locations": locations})
}

// BulkTagRequest represents a bulk tag operation request
type BulkTagRequest struct {
	EntryIDs []string `json:"entry_ids" binding:"required"`
	TagIDs   []string `json:"tag_ids" binding:"required"`
	Action   string   `json:"action" binding:"required,oneof=add remove"`
}

// BulkTag handles POST /api/v1/entries/bulk/tag
func (h *EntryHandler) BulkTag(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req BulkTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	var err error
	if req.Action == "add" {
		err = h.entryRepo.BulkAddTags(c.Request.Context(), req.EntryIDs, req.TagIDs)
	} else {
		err = h.entryRepo.BulkRemoveTags(c.Request.Context(), req.EntryIDs, req.TagIDs)
	}

	if err != nil {
		// Pass error directly - SendError handles AppError types
		SendError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "action": req.Action, "count": len(req.EntryIDs)})
}

// BulkDeleteRequest represents a bulk delete request
type BulkDeleteRequest struct {
	EntryIDs []string `json:"entry_ids" binding:"required"`
}

// BulkDelete handles POST /api/v1/entries/bulk/delete
func (h *EntryHandler) BulkDelete(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req BulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	err := h.entryRepo.BulkDelete(c.Request.Context(), userID, req.EntryIDs)
	if err != nil {
		// Pass error directly - SendError handles AppError types
		SendError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": len(req.EntryIDs)})
}

// BulkArchive handles POST /api/v1/entries/bulk/archive
func (h *EntryHandler) BulkArchive(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req BulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Queue all entries for archive refresh
	if h.archiveWorker != nil {
		h.archiveWorker.QueueJobs(req.EntryIDs)
	}

	c.JSON(http.StatusAccepted, gin.H{"queued": len(req.EntryIDs)})
}

// Create handles POST /api/v1/entries
func (h *EntryHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req CreateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	entry := &model.CatalogEntry{
		ID:            uuid.New().String(),
		UserID:        userID,
		URL:           req.URL,
		Title:         req.Title,
		Description:   req.Description,
		PhoneNumber:   req.PhoneNumber,
		Location:      req.Location,
		ArchiveStatus: model.ArchiveStatusPending,
	}

	err := h.entryRepo.Create(c.Request.Context(), entry)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	// Queue archive job if worker is available
	if h.archiveWorker != nil {
		h.archiveWorker.QueueJob(entry.ID)
	}

	c.JSON(http.StatusCreated, entryToResponse(entry, []TagInfo{}))
}

// Get handles GET /api/v1/entries/:id
func (h *EntryHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")
	entry, err := h.entryRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Fetch tags for the entry
	tags, err := h.tagRepo.GetEntryTags(id)
	tagInfos := make([]TagInfo, len(tags))
	if err == nil {
		for i, t := range tags {
			tagInfos[i] = TagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
		}
	}

	c.JSON(http.StatusOK, entryToResponse(entry, tagInfos))
}

// Update handles PUT /api/v1/entries/:id
func (h *EntryHandler) Update(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")

	// Get existing entry
	entry, err := h.entryRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	var req UpdateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Apply updates
	if req.Title != nil {
		entry.Title = *req.Title
	}
	if req.Description != nil {
		entry.Description = *req.Description
	}
	if req.PhoneNumber != nil {
		entry.PhoneNumber = *req.PhoneNumber
	}
	if req.Location != nil {
		entry.Location = *req.Location
	}
	if req.ThumbnailPath != nil {
		entry.ThumbnailPath = *req.ThumbnailPath
	}
	if req.ArchivePath != nil {
		entry.ArchivePath = *req.ArchivePath
	}
	if req.ArchiveStatus != nil {
		status := model.ArchiveStatus(*req.ArchiveStatus)
		entry.ArchiveStatus = status
	}

	err = h.entryRepo.Update(c.Request.Context(), entry)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	// Fetch tags for the entry
	tags, _ := h.tagRepo.GetEntryTags(id)
	tagInfos := make([]TagInfo, len(tags))
	for i, t := range tags {
		tagInfos[i] = TagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
	}

	c.JSON(http.StatusOK, entryToResponse(entry, tagInfos))
}

// Delete handles DELETE /api/v1/entries/:id
func (h *EntryHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")

	// Get entry first for file cleanup
	entry, err := h.entryRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Cascade delete archive files if storage config available
	if h.storageConfig != nil {
		// Delete archive directory for this entry
		archiveDir := filepath.Join(h.storageConfig.ArchivesDir, entry.ID)
		if err := os.RemoveAll(archiveDir); err != nil {
			// Log but don't fail the delete - DB delete is more important
			SendError(c, err) // Pass error directly
			return
		}

		// Delete thumbnail if exists (best effort, ignore errors)
		thumbDir := filepath.Join(h.storageConfig.ThumbDir, entry.ID)
		_ = os.RemoveAll(thumbDir)
	}

	err = h.entryRepo.Delete(c.Request.Context(), id, userID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.Status(http.StatusNoContent)
}

// RandomRequest represents the request for random entry selection
type RandomRequest struct {
	ExcludeTried bool     `json:"exclude_tried"`
	IncludeTags  []string `json:"include_tags"`
	ExcludeTags  []string `json:"exclude_tags"`
}

// Random handles POST /api/v1/entries/random
func (h *EntryHandler) Random(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req RandomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Default to empty request if body is missing
		req = RandomRequest{}
	}

	// Debug logging
	log.Printf("Random request: exclude_tried=%v, include_tags=%v, exclude_tags=%v", req.ExcludeTried, req.IncludeTags, req.ExcludeTags)

	var entry *model.CatalogEntry
	var err error

	// Determine which function to call based on filters
	hasTagFilter := len(req.IncludeTags) > 0

	if hasTagFilter {
		// Use the first tag for filtering (single tag selection from dropdown)
		tagID := req.IncludeTags[0]
		log.Printf("Picking random entry with tag filter: tagID=%s, excludeTried=%v", tagID, req.ExcludeTried)
		if req.ExcludeTried {
			entry, err = h.entryRepo.FindRandomTriedEntryWithTag(c.Request.Context(), userID, tagID)
		} else {
			entry, err = h.entryRepo.FindRandomEntryWithTag(c.Request.Context(), userID, tagID)
		}
	} else {
		// No tag filter
		log.Printf("Picking random entry without tag filter, excludeTried=%v", req.ExcludeTried)
		if req.ExcludeTried {
			entry, err = h.entryRepo.FindRandomTriedEntry(c.Request.Context(), userID)
		} else {
			entry, err = h.entryRepo.FindRandomEntry(c.Request.Context(), userID)
		}
	}

	if err != nil || entry == nil {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeNotFound, "no entries found matching criteria"))
		return
	}

	// Fetch tags for the entry
	tags, _ := h.tagRepo.GetEntryTags(entry.ID)
	tagInfos := make([]TagInfo, len(tags))
	for i, t := range tags {
		tagInfos[i] = TagInfo{ID: t.ID, Name: t.Name, Color: t.Color}
	}

	c.JSON(http.StatusOK, entryToResponse(entry, tagInfos))
}
