package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
)

// FacetsHandler handles the facets endpoint
type FacetsHandler struct {
	entryRepo *repository.EntryRepository
	tagRepo   *repository.TagRepository
	cfRepo    *repository.CustomFieldRepository
}

// NewFacetsHandler creates a new FacetsHandler
func NewFacetsHandler(entryRepo *repository.EntryRepository, tagRepo *repository.TagRepository, cfRepo *repository.CustomFieldRepository) *FacetsHandler {
	return &FacetsHandler{
		entryRepo: entryRepo,
		tagRepo:   tagRepo,
		cfRepo:    cfRepo,
	}
}

// Facets handles GET /api/v1/entries/facets
// Returns aggregated filter options (tags with counts, statuses with counts, custom field distributions, date range)
func (h *FacetsHandler) Facets(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	ctx := c.Request.Context()

	// Get base facets from entry repo
	result, err := h.entryRepo.Facets(ctx, userID)
	if err != nil {
		SendError(c, err)
		return
	}

	// Get custom field distributions
	fieldNames, err := h.cfRepo.GetDistinctFieldNames(ctx, userID)
	if err != nil {
		// Non-fatal — return facets without custom fields
		c.JSON(http.StatusOK, result)
		return
	}

	for _, fieldName := range fieldNames {
		values, err := h.cfRepo.GetFieldValueDistribution(ctx, userID, fieldName, 20)
		if err != nil {
			continue // Skip this field on error
		}
		if len(values) > 0 {
			result.CustomFields = append(result.CustomFields, model.CustomFieldFacet{
				FieldName: fieldName,
				Values:    values,
			})
		}
	}

	c.JSON(http.StatusOK, result)
}