package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
)

// InteractionHandler handles interaction HTTP requests
type InteractionHandler struct {
	interactionRepo *repository.InteractionRepository
	entryRepo       *repository.EntryRepository
}

// NewInteractionHandler creates a new InteractionHandler
func NewInteractionHandler(interactionRepo *repository.InteractionRepository, entryRepo *repository.EntryRepository) *InteractionHandler {
	return &InteractionHandler{
		interactionRepo: interactionRepo,
		entryRepo:       entryRepo,
	}
}

// InteractionResponse represents the API response for an interaction
type InteractionResponse struct {
	ID          string  `json:"id"`
	EntryID     string  `json:"entry_id"`
	UserID      string  `json:"user_id"`
	Tried       bool    `json:"tried"`
	Score       *int    `json:"score,omitempty"`
	Comments    string  `json:"comments,omitempty"`
	ContactedAt *string `json:"contacted_at,omitempty"`
	PurchasedAt *string `json:"purchased_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// Get handles GET /api/v1/entries/:id/interaction
func (h *InteractionHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	// Verify entry exists and belongs to user
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Get interaction
	interaction, err := h.interactionRepo.GetByEntryAndUser(c.Request.Context(), entryID, userID)
	if err != nil {
		// No interaction found - return empty response
		c.JSON(http.StatusOK, nil)
		return
	}

	c.JSON(http.StatusOK, interactionToResponse(interaction))
}

// Upsert handles PUT /api/v1/entries/:id/interaction
func (h *InteractionHandler) Upsert(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	// Verify entry exists and belongs to user
	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil || entry.UserID != userID {
		SendError(c, apperrors.NewNotFoundError(apperrors.CodeEntryNotFound, "entry"))
		return
	}

	// Parse input
	var input model.InteractionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	// Validate
	if !input.Validate() {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "score must be between 0 and 5"))
		return
	}

	// Upsert interaction
	interaction, err := h.interactionRepo.Upsert(c.Request.Context(), entryID, userID, &input)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.JSON(http.StatusOK, interactionToResponse(interaction))
}

// Delete handles DELETE /api/v1/entries/:id/interaction
func (h *InteractionHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	entryID := c.Param("id")

	err := h.interactionRepo.Delete(c.Request.Context(), entryID, userID)
	if err != nil {
		SendError(c, err) // Pass error directly
		return
	}

	c.Status(http.StatusNoContent)
}

func interactionToResponse(i *model.Interaction) *InteractionResponse {
	if i == nil {
		return nil
	}
	resp := &InteractionResponse{
		ID:        i.ID,
		EntryID:   i.EntryID,
		UserID:    i.UserID,
		Tried:     i.Tried,
		Score:     i.Score,
		Comments:  i.Comments,
		CreatedAt: i.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if i.ContactedAt != nil {
		t := i.ContactedAt.Format("2006-01-02T15:04:05Z")
		resp.ContactedAt = &t
	}
	if i.PurchasedAt != nil {
		t := i.PurchasedAt.Format("2006-01-02T15:04:05Z")
		resp.PurchasedAt = &t
	}
	return resp
}
