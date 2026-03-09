package handler

import (
	"net/http"
	"regexp"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TagHandler handles tag HTTP requests
type TagHandler struct {
	tagRepo *repository.TagRepository
}

// NewTagHandler creates a new TagHandler
func NewTagHandler(tagRepo *repository.TagRepository) *TagHandler {
	return &TagHandler{
		tagRepo: tagRepo,
	}
}

// Request/Response DTOs
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required,max=50"`
	Color string `json:"color"`
}

type UpdateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

type TagResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

type TagWithCountResponse struct {
	TagResponse
	Count int64 `json:"count"`
}

func tagToResponse(tag *model.Tag) *TagResponse {
	if tag == nil {
		return nil
	}
	return &TagResponse{
		ID:        tag.ID,
		UserID:    tag.UserID,
		Name:      tag.Name,
		Color:     tag.Color,
		CreatedAt: tag.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// hexColorRegex validates hex color format
var hexColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// List handles GET /api/v1/tags
func (h *TagHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	tags, err := h.tagRepo.FindByUserID(userID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	result := make([]*TagResponse, len(tags))
	for i, t := range tags {
		result[i] = tagToResponse(&t)
	}

	c.JSON(http.StatusOK, result)
}

// ListWithCount handles GET /api/v1/tags?with_count=true
func (h *TagHandler) ListWithCount(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	tags, err := h.tagRepo.GetTagsWithCount(userID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	result := make([]*TagWithCountResponse, len(tags))
	for i, t := range tags {
		result[i] = &TagWithCountResponse{
			TagResponse: *tagToResponse(&t.Tag),
			Count:       t.Count,
		}
	}

	c.JSON(http.StatusOK, result)
}

// Create handles POST /api/v1/tags
func (h *TagHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Validate color format if provided
	if req.Color != "" && !hexColorRegex.MatchString(req.Color) {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, "invalid color format: must be hex color (e.g. #FF5733)"))
		return
	}

	// Set default color if not provided
	if req.Color == "" {
		req.Color = "#6B7280"
	}

	tag := &model.Tag{
		ID:    uuid.New().String(),
		UserID: userID,
		Name:  req.Name,
		Color: req.Color,
	}

	err := h.tagRepo.Create(tag)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.JSON(http.StatusCreated, tagToResponse(tag))
}

// Get handles GET /api/v1/tags/:id
func (h *TagHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")
	tag, err := h.tagRepo.FindByID(id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if tag == nil || tag.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeTagNotFound, "tag"))
		return
	}

	c.JSON(http.StatusOK, tagToResponse(tag))
}

// Update handles PUT /api/v1/tags/:id
func (h *TagHandler) Update(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")

	// Get existing tag
	tag, err := h.tagRepo.FindByID(id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if tag == nil || tag.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeTagNotFound, "tag"))
		return
	}

	var req UpdateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Apply updates
	if req.Name != nil {
		tag.Name = *req.Name
	}
	if req.Color != nil {
		if !hexColorRegex.MatchString(*req.Color) {
			SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, "invalid color format: must be hex color (e.g. #FF5733)"))
			return
		}
		tag.Color = *req.Color
	}

	err = h.tagRepo.Update(tag)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.JSON(http.StatusOK, tagToResponse(tag))
}

// Delete handles DELETE /api/v1/tags/:id
func (h *TagHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	id := c.Param("id")

	// Verify ownership
	tag, err := h.tagRepo.FindByID(id)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if tag == nil || tag.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeTagNotFound, "tag"))
		return
	}

	err = h.tagRepo.Delete(id)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.Status(http.StatusNoContent)
}

// AddEntryTag handles POST /api/v1/entries/:id/tags
func (h *TagHandler) AddEntryTag(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	// Get tag ID from request body
	var req struct {
		TagID string `json:"tag_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Verify tag belongs to user
	tag, err := h.tagRepo.FindByID(req.TagID)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if tag == nil || tag.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeTagNotFound, "tag"))
		return
	}

	err = h.tagRepo.AddEntryTag(entryID, req.TagID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tag added to entry"})
}

// RemoveEntryTag handles DELETE /api/v1/entries/:id/tags/:tag_id
func (h *TagHandler) RemoveEntryTag(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")
	tagID := c.Param("tag_id")

	// Verify tag belongs to user
	tag, err := h.tagRepo.FindByID(tagID)
	if err != nil {
		// SendError handles AppError types (NotFoundError, etc.)
		SendError(c, err)
		return
	}
	if tag == nil || tag.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeTagNotFound, "tag"))
		return
	}

	err = h.tagRepo.RemoveEntryTag(entryID, tagID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.Status(http.StatusNoContent)
}
