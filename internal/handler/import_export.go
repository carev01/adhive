package handler

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/carev01/adhive/internal/errors"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
)

// ImportExportHandler handles bulk import and export operations
type ImportExportHandler struct {
	entryRepo *repository.EntryRepository
	tagRepo   *repository.TagRepository
}

// NewImportExportHandler creates a new ImportExportHandler
func NewImportExportHandler(entryRepo *repository.EntryRepository, tagRepo *repository.TagRepository) *ImportExportHandler {
	return &ImportExportHandler{
		entryRepo: entryRepo,
		tagRepo:   tagRepo,
	}
}

// Import handles POST /api/v1/entries/import
func (h *ImportExportHandler) Import(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "no file uploaded or invalid form data"))
		return
	}
	defer file.Close()

	filename := strings.ToLower(header.Filename)
	var rows []service.ImportRow
	var parseErrors []service.RowError

	if strings.HasSuffix(filename, ".csv") {
		rows, parseErrors, err = service.ParseCSV(file)
		if err != nil {
			SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, err.Error()))
			return
		}
	} else if strings.HasSuffix(filename, ".json") {
		rows, parseErrors, err = service.ParseJSON(file)
		if err != nil {
			SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, err.Error()))
			return
		}
	} else {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, "unsupported file format. Use .csv or .json"))
		return
	}

	if len(rows) == 0 {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput, "file contains no data rows"))
		return
	}

	const maxBatchSize = 10000
	if len(rows) > maxBatchSize {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidInput,
			fmt.Sprintf("file contains %d rows, maximum is %d", len(rows), maxBatchSize)))
		return
	}

	ctx := c.Request.Context()

	urlSet := make(map[string]bool)
	for i, row := range rows {
		if service.ValidateRow(row, i+2) == nil {
			urlSet[row.URL] = true
		}
	}

	urls := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urls = append(urls, u)
	}
	existingMap, err := h.entryRepo.GetByURLs(ctx, userID, urls)
	if err != nil {
		log.Printf("Import: error looking up existing entries: %v", err)
		SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to look up existing entries", err))
		return
	}

	var creates []*model.CatalogEntry
	var updates []*model.CatalogEntry
	result := service.ImportResult{
		SkippedRows: []service.SkippedRow{},
		Errors:      []service.RowError{},
	}

	seenURLs := make(map[string]bool)
	result.Errors = append(result.Errors, parseErrors...)
	result.ErrorCount += len(parseErrors)

	for i, row := range rows {
		rowNum := i + 2

		if validationErr := service.ValidateRow(row, rowNum); validationErr != nil {
			result.Errors = append(result.Errors, *validationErr)
			result.ErrorCount++
			continue
		}

		if seenURLs[row.URL] {
			result.SkippedRows = append(result.SkippedRows, service.SkippedRow{Row: rowNum, URL: row.URL})
			result.SkippedCount++
			continue
		}
		seenURLs[row.URL] = true

		if existing, found := existingMap[row.URL]; found {
			updated := service.MergeEntry(existing, row)
			updates = append(updates, updated)
			result.UpdatedCount++
		} else {
			newEntry := service.RowToEntry(row, userID)
			creates = append(creates, newEntry)
			result.ImportedCount++
		}
	}

	if len(creates) > 0 || len(updates) > 0 {
		err := h.entryRepo.BatchUpsertByURL(ctx, userID, creates, buildUpdatesMap(updates))
		if err != nil {
			log.Printf("Import: batch operation failed: %v", err)
			SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "import batch operation failed", err))
			return
		}
	}

	c.JSON(http.StatusOK, result)
}

func buildUpdatesMap(updates []*model.CatalogEntry) map[string]*model.CatalogEntry {
	m := make(map[string]*model.CatalogEntry, len(updates))
	for _, e := range updates {
		m[e.ID] = e
	}
	return m
}

// Export handles GET /api/v1/entries/export
func (h *ImportExportHandler) Export(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		SendError(c, apperrors.NewUnauthorizedError(apperrors.CodeUnauthorized, "user not authenticated"))
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format != "csv" && format != "json" {
		SendError(c, apperrors.NewValidationError(apperrors.CodeInvalidFormat, "format must be 'csv' or 'json'"))
		return
	}

	ctx := c.Request.Context()
	filter := buildExportFilter(c)

	entries, err := h.entryRepo.ExportByUserID(ctx, userID, filter)
	if err != nil {
		log.Printf("Export: error fetching entries: %v", err)
		SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to fetch entries for export", err))
		return
	}

	if format == "json" {
		data, err := service.EntriesToJSON(entries)
		if err != nil {
			SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to generate JSON export", err))
			return
		}
		c.Header("Content-Disposition", "attachment; filename=adhive-export.json")
		c.Data(http.StatusOK, "application/json", data)
		return
	}

	data, err := service.EntriesToCSV(entries)
	if err != nil {
		SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to generate CSV export", err))
		return
	}
	c.Header("Content-Disposition", "attachment; filename=adhive-export.csv")
	c.Data(http.StatusOK, "text/csv", data)
}

// Template handles GET /api/v1/entries/template
func (h *ImportExportHandler) Template(c *gin.Context) {
	data, err := service.GenerateCSVTemplate()
	if err != nil {
		SendError(c, apperrors.NewInternalError(apperrors.CodeInternal, "failed to generate template", err))
		return
	}
	c.Header("Content-Disposition", "attachment; filename=adhive-import-template.csv")
	c.Data(http.StatusOK, "text/csv", data)
}

func buildExportFilter(c *gin.Context) *model.EntryFilter {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 100000 // Export all, no pagination
	tagID := c.Query("tag")
	status := c.Query("status")
	search := c.Query("search")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	source := c.Query("source")
	location := c.Query("location")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	excludeTried := c.Query("exclude_tried") == "true"
	hasInteraction := c.Query("has_interaction") == "true"
	minScore, _ := strconv.Atoi(c.Query("min_score"))

	if page < 1 {
		page = 1
	}

	filter := &model.EntryFilter{
		Page:           page,
		Limit:          limit,
		TagID:          tagID,
		Search:         search,
		ExcludeTried:   excludeTried,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		Source:          source,
		Location:       location,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		HasInteraction:  hasInteraction,
		MinScore:        minScore,
	}
	if status != "" {
		filter.Status = model.ArchiveStatus(status)
	}
	return filter
}

func parseImportFile(file multipart.File, header *multipart.FileHeader) ([]service.ImportRow, []service.RowError, string, error) {
	filename := strings.ToLower(header.Filename)
	if strings.HasSuffix(filename, ".csv") {
		rows, errors, err := service.ParseCSV(file)
		return rows, errors, "csv", err
	}
	if strings.HasSuffix(filename, ".json") {
		rows, errors, err := service.ParseJSON(file)
		return rows, errors, "json", err
	}
	return nil, nil, "", fmt.Errorf("unsupported file format: %s. Use .csv or .json", header.Filename)
}

func parseImportBuffer(data []byte, filename string) ([]service.ImportRow, []service.RowError, error) {
	reader := bytes.NewReader(data)
	filename = strings.ToLower(filename)
	if strings.HasSuffix(filename, ".csv") {
		return service.ParseCSV(reader)
	}
	if strings.HasSuffix(filename, ".json") {
		return service.ParseJSON(reader)
	}
	return nil, nil, fmt.Errorf("unsupported file format: %s", filename)
}