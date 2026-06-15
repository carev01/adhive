package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
)

// CustomFieldHandler handles custom field HTTP requests
type CustomFieldHandler struct {
	cfRepo    *repository.CustomFieldRepository
	entryRepo *repository.EntryRepository
}

// NewCustomFieldHandler creates a new CustomFieldHandler
func NewCustomFieldHandler(cfRepo *repository.CustomFieldRepository, entryRepo *repository.EntryRepository) *CustomFieldHandler {
	return &CustomFieldHandler{
		cfRepo:    cfRepo,
		entryRepo: entryRepo,
	}
}

func customFieldToResponse(cf *model.CustomField) *model.CustomFieldResponse {
	if cf == nil {
		return nil
	}
	return &model.CustomFieldResponse{
		ID:         cf.ID,
		EntryID:    cf.EntryID,
		FieldName:  cf.FieldName,
		FieldValue: cf.FieldValue,
		CreatedAt:  cf.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  cf.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ListEntryCustomFields handles GET /api/v1/entries/:id/custom_fields
func (h *CustomFieldHandler) ListEntryCustomFields(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	// Verify entry ownership
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	fields, err := h.cfRepo.GetByEntryID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err)
		return
	}

	responses := make([]*model.CustomFieldResponse, len(fields))
	for i, f := range fields {
		responses[i] = customFieldToResponse(&f)
	}

	c.JSON(http.StatusOK, gin.H{"custom_fields": responses})
}

// CreateEntryCustomField handles POST /api/v1/entries/:id/custom_fields
func (h *CustomFieldHandler) CreateEntryCustomField(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	// Verify entry ownership
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	var req model.CustomFieldInput
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	cf := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entryID,
		FieldName:  req.FieldName,
		FieldValue: req.FieldValue,
	}

	if err := h.cfRepo.Create(c.Request.Context(), cf); err != nil {
		SendError(c, err)
		return
	}

	c.JSON(http.StatusCreated, customFieldToResponse(cf))
}

// UpdateEntryCustomField handles PUT /api/v1/entries/:id/custom_fields/:fieldId
func (h *CustomFieldHandler) UpdateEntryCustomField(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")
	fieldID := c.Param("fieldId")

	// Verify entry ownership
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Get existing field
	cf, err := h.cfRepo.GetByID(c.Request.Context(), fieldID)
	if err != nil {
		SendError(c, err)
		return
	}
	if cf == nil || cf.EntryID != entryID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeNotFound, "custom field"))
		return
	}

	var req model.CustomFieldUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	cf.FieldValue = req.FieldValue
	if err := h.cfRepo.Update(c.Request.Context(), cf); err != nil {
		SendError(c, err)
		return
	}

	c.JSON(http.StatusOK, customFieldToResponse(cf))
}

// DeleteEntryCustomField handles DELETE /api/v1/entries/:id/custom_fields/:fieldId
func (h *CustomFieldHandler) DeleteEntryCustomField(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")
	fieldID := c.Param("fieldId")

	// Verify entry ownership
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil {
		SendError(c, err)
		return
	}
	if entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Verify field belongs to this entry
	cf, err := h.cfRepo.GetByID(c.Request.Context(), fieldID)
	if err != nil {
		SendError(c, err)
		return
	}
	if cf == nil || cf.EntryID != entryID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeNotFound, "custom field"))
		return
	}

	if err := h.cfRepo.Delete(c.Request.Context(), fieldID); err != nil {
		SendError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}