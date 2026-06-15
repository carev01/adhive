package repository

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/carev01/adhive/internal/model"

	"gorm.io/gorm"
)

// EntryRepository handles entry database operations
type EntryRepository struct {
	db *gorm.DB
}

// NewEntryRepository creates a new EntryRepository
func NewEntryRepository(db *gorm.DB) *EntryRepository {
	return &EntryRepository{db: db}
}

// Create creates a new entry
func (r *EntryRepository) Create(ctx context.Context, entry *model.CatalogEntry) error {
	err := r.db.WithContext(ctx).Create(entry).Error
	if err != nil {
		return WrapDBError(err, "Entry", entry.ID)
	}
	return nil
}

// GetByID finds an entry by ID
func (r *EntryRepository) GetByID(ctx context.Context, id string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
	if err != nil {
		return nil, WrapDBError(err, "Entry", id)
	}
	return &entry, nil
}

// applyCommonFilters applies all non-search filters to a query.
// This is shared between GetByUserID and Search methods.
func applyCommonFilters(query *gorm.DB, userID string, filter *model.EntryFilter) *gorm.DB {
	// Multi-tag AND filtering: entry must have ALL specified tags
	if len(filter.Tags) > 0 {
		if filter.TagsLogic == "or" {
			// OR: entry has ANY of the specified tags
			query = query.Where("catalog_entries.id IN (SELECT et.entry_id FROM entry_tags et WHERE et.tag_id IN ?)", filter.Tags)
		} else {
			// AND (default): entry has ALL specified tags
			for _, tagID := range filter.Tags {
				query = query.Where("catalog_entries.id IN (SELECT et.entry_id FROM entry_tags et WHERE et.tag_id = ?)", tagID)
			}
		}
	}

	// Legacy single-tag filter (backward compat)
	if filter.TagID != "" && len(filter.Tags) == 0 {
		query = query.Joins("JOIN entry_tags ON catalog_entries.id = entry_tags.entry_id").
			Where("entry_tags.tag_id = ?", filter.TagID)
	}

	if filter.Status != "" {
		query = query.Where("archive_status = ?", filter.Status)
	}

	if filter.ExcludeTried {
		query = query.Where("id NOT IN (SELECT entry_id FROM interactions WHERE user_id = ? AND tried = ?)", userID, true)
	}

	if filter.HasInteraction {
		query = query.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ?)", userID)
	}

	if filter.MinScore > 0 {
		query = query.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ? AND score >= ?)", userID, filter.MinScore)
	}

	if filter.DateFrom != "" {
		query = query.Where("created_at >= ?", filter.DateFrom)
	}
	if filter.DateTo != "" {
		query = query.Where("created_at <= ?", filter.DateTo+" 23:59:59")
	}

	if filter.Source != "" {
		query = query.Where("url LIKE ?", "%"+filter.Source+"%")
	}

	if filter.Location != "" {
		query = query.Where("location LIKE ?", "%"+filter.Location+"%")
	}

	// Custom field EAV filtering
	for fieldName, fieldValue := range filter.CustomFields {
		query = query.Where("catalog_entries.id IN (SELECT cf.entry_id FROM custom_fields cf WHERE cf.field_name = ? AND cf.field_value = ?)", fieldName, fieldValue)
	}

	return query
}

// applySortOrder determines sort field and direction from filter
func applySortOrder(filter *model.EntryFilter, tablePrefix string) string {
	sortField := "created_at"
	sortDir := "DESC"
	if tablePrefix != "" {
		sortField = tablePrefix + "." + sortField
	}
	switch filter.SortBy {
	case "title":
		if tablePrefix != "" {
			sortField = tablePrefix + ".title"
		} else {
			sortField = "title"
		}
	case "updated_at":
		if tablePrefix != "" {
			sortField = tablePrefix + ".updated_at"
		} else {
			sortField = "updated_at"
		}
	}
	if filter.SortOrder == "asc" {
		sortDir = "ASC"
	}
	return sortField + " " + sortDir
}

// GetByUserID finds all entries for a user with pagination and filtering
func (r *EntryRepository) GetByUserID(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	// Apply all common filters
	query = applyCommonFilters(query, userID, filter)

	// Count total
	var total int64
	query.Model(&model.CatalogEntry{}).Count(&total)

	// Apply pagination
	offset := (filter.Page - 1) * filter.Limit
	var entries []*model.CatalogEntry
	err := query.Order(applySortOrder(filter, "")).
		Offset(offset).
		Limit(filter.Limit).
		Find(&entries).Error
	if err != nil {
		return nil, err
	}

	return &model.EntryListResult{
		Entries: entries,
		Total:   total,
		Page:    filter.Page,
		Limit:   filter.Limit,
	}, nil
}

// Search performs full-text search on entries using FTS5 for better performance
func (r *EntryRepository) Search(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	// First try FTS5 search
	result, err := r.searchWithFTS5(ctx, userID, query, filter)

	// If FTS5 fails or returns no results, fall back to LIKE search
	if err != nil || (result != nil && result.Total == 0) {
		log.Printf("FTS5 search failed or returned no results (%v), falling back to LIKE search", err)
		return r.searchWithLike(ctx, userID, query, filter)
	}

	return result, nil
}

// searchWithFTS5 performs full-text search using FTS5
func (r *EntryRepository) searchWithFTS5(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	// Use FTS5 for full-text search (much faster than LIKE)
	// First find matching rowids from FTS, then join with catalog_entries
	ftsQuery := r.db.WithContext(ctx).
		Table("entries_fts fts").
		Select("catalog_entries.*").
		Joins("JOIN catalog_entries ON catalog_entries.rowid = fts.rowid").
		Where("catalog_entries.user_id = ?", userID).
		Where("entries_fts MATCH ?", query+"*")

	// Apply all common filters using the same function
	ftsQuery = applyCommonFilters(ftsQuery, userID, filter)

	// Determine sort order
	orderClause := applySortOrder(filter, "catalog_entries")

	// Count total matching entries
	var total int64
	ftsQuery.Count(&total)

	// Apply pagination and get entry IDs first
	offset := (filter.Page - 1) * filter.Limit
	var entryIDs []string
	err := r.db.WithContext(ctx).
		Table("entries_fts fts").
		Select("catalog_entries.id").
		Joins("JOIN catalog_entries ON catalog_entries.rowid = fts.rowid").
		Where("catalog_entries.user_id = ?", userID).
		Where("entries_fts MATCH ?", query+"*").
		Order(orderClause).
		Offset(offset).
		Limit(filter.Limit).
		Pluck("id", &entryIDs).Error

	if err != nil {
		return nil, err
	}

	// Now fetch the full entries (tags are fetched separately by handler)
	var entries []*model.CatalogEntry
	if len(entryIDs) > 0 {
		err = r.db.WithContext(ctx).
			Where("id IN ?", entryIDs).
			Order(orderClause).
			Find(&entries).Error
		if err != nil {
			return nil, err
		}
	}

	return &model.EntryListResult{
		Entries: entries,
		Total:   total,
		Page:    filter.Page,
		Limit:   filter.Limit,
	}, nil
}

// searchWithLike performs fallback search using LIKE
func (r *EntryRepository) searchWithLike(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	searchPattern := "%" + query + "%"

	// Build base query with LIKE
	baseQuery := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(phone_number) LIKE ? OR LOWER(location) LIKE ? OR LOWER(url) LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)

	// Apply all common filters
	baseQuery = applyCommonFilters(baseQuery, userID, filter)

	// Sort
	orderClause := applySortOrder(filter, "")

	// Count total
	var total int64
	baseQuery.Model(&model.CatalogEntry{}).Count(&total)

	// Apply pagination
	offset := (filter.Page - 1) * filter.Limit
	var entries []*model.CatalogEntry
	err := baseQuery.
		Order(orderClause).
		Offset(offset).
		Limit(filter.Limit).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}

	return &model.EntryListResult{
		Entries: entries,
		Total:   total,
		Page:    filter.Page,
		Limit:   filter.Limit,
	}, nil
}

// Update updates an existing entry
func (r *EntryRepository) Update(ctx context.Context, entry *model.CatalogEntry) error {
	err := r.db.WithContext(ctx).Save(entry).Error
	if err != nil {
		return WrapDBError(err, "Entry", entry.ID)
	}
	return nil
}

// Delete deletes an entry by ID (only if owned by user)
func (r *EntryRepository) Delete(ctx context.Context, id, userID string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First verify ownership
		var entry model.CatalogEntry
		if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&entry).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}

		// Get revision IDs for this entry
		var revisionIDs []string
		if err := tx.Model(&model.ArchiveRevision{}).Where("entry_id = ?", id).Pluck("id", &revisionIDs).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}

		// Delete archive assets by revision IDs
		if len(revisionIDs) > 0 {
			if err := tx.Where("revision_id IN ?", revisionIDs).Delete(&model.ArchiveAsset{}).Error; err != nil {
				return WrapDBError(err, "Entry", id)
			}
		}
		// Delete archive revisions
		if err := tx.Where("entry_id = ?", id).Delete(&model.ArchiveRevision{}).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}
		// Delete entry tags
		if err := tx.Where("entry_id = ?", id).Delete(&model.EntryTag{}).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}
		// Delete custom fields
		if err := tx.Where("entry_id = ?", id).Delete(&model.CustomField{}).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}
		// Delete interactions
		if err := tx.Where("entry_id = ?", id).Delete(&model.Interaction{}).Error; err != nil {
			return WrapDBError(err, "Entry", id)
		}
		// Delete entry
		return tx.Where("id = ?", id).Delete(&model.CatalogEntry{}).Error
	})
	return WrapDBError(err, "Entry", id)
}

// GetByUserIDWithTags finds all entries for a user with their tags (fixes N+1 query)
func (r *EntryRepository) GetByUserIDWithTags(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	// Build query with preloaded tags
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	// Apply all common filters
	query = applyCommonFilters(query, userID, filter)

	// Count total
	var total int64
	query.Model(&model.CatalogEntry{}).Count(&total)

	// Build dynamic ORDER BY clause
	orderClause := applySortOrder(filter, "")

	// Apply pagination (tags fetched separately by handler)
	offset := (filter.Page - 1) * filter.Limit
	var entries []*model.CatalogEntry
	err := query.
		Order(orderClause).
		Offset(offset).
		Limit(filter.Limit).
		Find(&entries).Error
	if err != nil {
		return nil, err
	}

	return &model.EntryListResult{
		Entries: entries,
		Total:   total,
		Page:    filter.Page,
		Limit:   filter.Limit,
	}, nil
}

// GetEntriesTags fetches tags for multiple entries in a single query (batch fetch to fix N+1)
func (r *EntryRepository) GetEntriesTags(ctx context.Context, entryIDs []string) (map[string][]model.Tag, error) {
	if len(entryIDs) == 0 {
		return map[string][]model.Tag{}, nil
	}

	type entryTagRow struct {
		EntryID string `gorm:"column:entry_id"`
		model.Tag
	}

	var rows []entryTagRow
	err := r.db.WithContext(ctx).
		Table("entry_tags").
		Select("entry_tags.entry_id, tags.id, tags.user_id, tags.name, tags.color, tags.created_at").
		Joins("JOIN tags ON tags.id = entry_tags.tag_id").
		Where("entry_tags.entry_id IN ?", entryIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string][]model.Tag, len(entryIDs))
	for _, row := range rows {
		result[row.EntryID] = append(result[row.EntryID], row.Tag)
	}
	return result, nil
}

// Facets returns aggregated filter options for the facets endpoint
func (r *EntryRepository) Facets(ctx context.Context, userID string) (*model.FacetResult, error) {
	result := &model.FacetResult{}

	// Tag counts
	var tagFacets []model.TagFacet
	err := r.db.WithContext(ctx).
		Table("tags").
		Select("tags.id, tags.name, tags.color, COUNT(entry_tags.entry_id) as count").
		Joins("LEFT JOIN entry_tags ON tags.id = entry_tags.tag_id").
		Joins("LEFT JOIN catalog_entries ON entry_tags.entry_id = catalog_entries.id AND catalog_entries.user_id = ?", userID).
		Where("tags.user_id = ?", userID).
		Group("tags.id, tags.name, tags.color").
		Order("tags.name ASC").
		Find(&tagFacets).Error
	if err != nil {
		return nil, WrapDBError(err, "Facet", "")
	}
	result.Tags = tagFacets

	// Status counts
	var statusFacets []model.StatusFacet
	err = r.db.WithContext(ctx).
		Table("catalog_entries").
		Select("archive_status as status, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("archive_status").
		Order("count DESC").
		Find(&statusFacets).Error
	if err != nil {
		return nil, WrapDBError(err, "Facet", "")
	}
	result.Statuses = statusFacets

	// Date range
	var dateRange model.DateRangeFacet
	err = r.db.WithContext(ctx).
		Table("catalog_entries").
		Select("MIN(created_at) as min, MAX(created_at) as max").
		Where("user_id = ?", userID).
		Find(&dateRange).Error
	if err != nil {
		return nil, WrapDBError(err, "Facet", "")
	}
	if dateRange.Min != "" || dateRange.Max != "" {
		result.DateRange = &dateRange
	}

	return result, nil
}

// AddTag adds a tag to an entry
func (r *EntryRepository) AddTag(ctx context.Context, entryID, tagID string) error {
	entryTag := model.EntryTag{
		EntryID: entryID,
		TagID:   tagID,
	}
	return r.db.WithContext(ctx).Create(&entryTag).Error
}

// RemoveTag removes a tag from an entry
func (r *EntryRepository) RemoveTag(ctx context.Context, entryID, tagID string) error {
	return r.db.WithContext(ctx).Where("entry_id = ? AND tag_id = ?", entryID, tagID).Delete(&model.EntryTag{}).Error
}

// BulkAddTags adds tags to multiple entries
func (r *EntryRepository) BulkAddTags(ctx context.Context, entryIDs, tagIDs []string) error {
	if len(entryIDs) == 0 || len(tagIDs) == 0 {
		return nil
	}

	var entryTags []model.EntryTag
	for _, entryID := range entryIDs {
		for _, tagID := range tagIDs {
			entryTags = append(entryTags, model.EntryTag{
				EntryID: entryID,
				TagID:   tagID,
			})
		}
	}

	return r.db.WithContext(ctx).Create(&entryTags).Error
}

// BulkRemoveTags removes tags from multiple entries
func (r *EntryRepository) BulkRemoveTags(ctx context.Context, entryIDs, tagIDs []string) error {
	if len(entryIDs) == 0 || len(tagIDs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).
		Where("entry_id IN ?", entryIDs).
		Where("tag_id IN ?", tagIDs).
		Delete(&model.EntryTag{}).Error
}

// BulkDelete deletes multiple entries
func (r *EntryRepository) BulkDelete(ctx context.Context, userID string, entryIDs []string) error {
	if len(entryIDs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Verify ownership for all entries
		var count int64
		if err := tx.Model(&model.CatalogEntry{}).
			Where("id IN ?", entryIDs).
			Where("user_id = ?", userID).
			Count(&count).Error; err != nil {
			return err
		}

		if count != int64(len(entryIDs)) {
			return errors.New("one or more entries not found or not owned by user")
		}

		// Delete entry_tag associations
		if err := tx.Where("entry_id IN ?", entryIDs).Delete(&model.EntryTag{}).Error; err != nil {
			return err
		}

		// Delete custom fields
		if err := tx.Where("entry_id IN ?", entryIDs).Delete(&model.CustomField{}).Error; err != nil {
			return err
		}

		// Delete interactions
		if err := tx.Where("entry_id IN ?", entryIDs).Delete(&model.Interaction{}).Error; err != nil {
			return err
		}

		// Delete entries
		if err := tx.Where("id IN ?", entryIDs).Delete(&model.CatalogEntry{}).Error; err != nil {
			return err
		}

		return nil
	})
}

// FindRandomEntry finds a random entry for a user using count + offset (O(1) vs O(n log n))
func (r *EntryRepository) FindRandomEntry(ctx context.Context, userID string) (*model.CatalogEntry, error) {
	// Get total count for this user
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.CatalogEntry{}).
		Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, WrapDBError(err, "Entry", "")
	}

	if count == 0 {
		return nil, WrapDBError(gorm.ErrRecordNotFound, "Entry", "")
	}

	// Random offset
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomOffset := rng.Intn(int(count))

	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Offset(randomOffset).
		First(&entry).Error

	return &entry, err
}

// FindRandomTriedEntry finds a random entry that hasn't been tried yet using count + offset
func (r *EntryRepository) FindRandomTriedEntry(ctx context.Context, userID string) (*model.CatalogEntry, error) {
	// Get count of untried entries
	var count int64
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM catalog_entries ce
		LEFT JOIN interactions i ON ce.id = i.entry_id AND i.user_id = ?
		WHERE ce.user_id = ? AND (i.id IS NULL OR i.tried = 0)`, userID, userID).Scan(&count).Error
	if err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Random offset
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomOffset := rng.Intn(int(count))

	var entry model.CatalogEntry
	err = r.db.WithContext(ctx).Raw(`
		SELECT ce.* FROM catalog_entries ce
		LEFT JOIN interactions i ON ce.id = i.entry_id AND i.user_id = ?
		WHERE ce.user_id = ? AND (i.id IS NULL OR i.tried = 0)
		LIMIT 1 OFFSET ?`, userID, userID, randomOffset).
		Scan(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// FindRandomEntryWithTag finds a random entry with a specific tag using count + offset
func (r *EntryRepository) FindRandomEntryWithTag(ctx context.Context, userID, tagID string) (*model.CatalogEntry, error) {
	// Get count of entries with this tag using raw SQL for reliability
	var count int64
	r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM catalog_entries ce
		JOIN entry_tags et ON ce.id = et.entry_id
		WHERE ce.user_id = ? AND et.tag_id = ?`, userID, tagID).Scan(&count)

	if count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Random offset
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomOffset := rng.Intn(int(count))

	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Raw(`
		SELECT ce.* FROM catalog_entries ce
		JOIN entry_tags et ON ce.id = et.entry_id
		WHERE ce.user_id = ? AND et.tag_id = ?
		LIMIT 1 OFFSET ?`, userID, tagID, randomOffset).
		Scan(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// FindRandomTriedEntryWithTag finds a random entry with a specific tag that hasn't been tried using count + offset
func (r *EntryRepository) FindRandomTriedEntryWithTag(ctx context.Context, userID, tagID string) (*model.CatalogEntry, error) {
	// Get count of untried entries with this tag using raw SQL for reliability
	var count int64
	r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM catalog_entries ce
		JOIN entry_tags et ON ce.id = et.entry_id
		LEFT JOIN interactions i ON ce.id = i.entry_id AND i.user_id = ?
		WHERE ce.user_id = ? AND et.tag_id = ? AND (i.id IS NULL OR i.tried = 0)`, userID, userID, tagID).Scan(&count)

	if count == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Random offset
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomOffset := rng.Intn(int(count))

	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Raw(`
		SELECT ce.* FROM catalog_entries ce
		JOIN entry_tags et ON ce.id = et.entry_id
		LEFT JOIN interactions i ON ce.id = i.entry_id AND i.user_id = ?
		WHERE ce.user_id = ? AND et.tag_id = ? AND (i.id IS NULL OR i.tried = 0)
		LIMIT 1 OFFSET ?`, userID, userID, tagID, randomOffset).
		Scan(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// FindByURL finds an entry by URL for a specific user
func (r *EntryRepository) FindByURL(ctx context.Context, userID, url string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("user_id = ? AND url = ?", userID, url).First(&entry).Error
	if err != nil {
		return nil, WrapDBError(err, "Entry", "")
	}
	return &entry, nil
}

// CountByUserID counts entries for a user
func (r *EntryRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.CatalogEntry{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// FindPendingForArchiving finds entries pending archival (excludes imported entries)
func (r *EntryRepository) FindPendingForArchiving(ctx context.Context, limit int) ([]*model.CatalogEntry, error) {
	var entries []*model.CatalogEntry
	err := r.db.WithContext(ctx).
		Where("archive_status = ?", model.ArchiveStatusPending).
		Or("archive_status = ? AND archive_path = ?", model.ArchiveStatusFailed, "").
		Where("(imported_from IS NULL OR imported_from = '')").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// GetUserSources returns unique domains/sources from user's entries
func (r *EntryRepository) GetUserSources(ctx context.Context, userID string) ([]string, error) {
	var results []string

	// Extract domain from URL using a simple approach
	rows, err := r.db.WithContext(ctx).
		Model(&model.CatalogEntry{}).
		Select("DISTINCT SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) as source").
		Where("user_id = ?", userID).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err == nil && source != "" {
			results = append(results, source)
		}
	}

	return results, nil
}

// GetUserLocations returns unique locations from user's entries
func (r *EntryRepository) GetUserLocations(ctx context.Context, userID string) ([]string, error) {
	var results []string

	rows, err := r.db.WithContext(ctx).
		Model(&model.CatalogEntry{}).
		Select("DISTINCT location").
		Where("user_id = ?", userID).
		Where("location IS NOT NULL AND location != ''").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var location string
		if err := rows.Scan(&location); err == nil && location != "" {
			results = append(results, location)
		}
	}

	// Sort alphabetically
	sort.Strings(results)

	return results, nil
}