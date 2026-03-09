package repository

import (
	"context"
	"errors"
	"sort"

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
	return r.db.WithContext(ctx).Create(entry).Error
}

// GetByID finds an entry by ID
func (r *EntryRepository) GetByID(ctx context.Context, id string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetByUserID finds all entries for a user with pagination
func (r *EntryRepository) GetByUserID(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	// Apply filters
	if filter.TagID != "" {
		query = query.Joins("JOIN entry_tags ON catalog_entries.id = entry_tags.entry_id").
			Where("entry_tags.tag_id = ?", filter.TagID)
	}

	if filter.Status != "" {
		query = query.Where("archive_status = ?", filter.Status)
	}

	if filter.ExcludeTried {
		query = query.Where("id NOT IN (SELECT entry_id FROM interactions WHERE user_id = ? AND tried = ?)", userID, true)
	}

	// HasInteraction filter - only entries with interactions
	if filter.HasInteraction {
		query = query.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ?)", userID)
	}

	// MinScore filter - entries where user's score >= minScore (implies HasInteraction)
	if filter.MinScore > 0 {
		query = query.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ? AND score >= ?)", userID, filter.MinScore)
	}

	// Date range filter
	if filter.DateFrom != "" {
		query = query.Where("created_at >= ?", filter.DateFrom)
	}
	if filter.DateTo != "" {
		query = query.Where("created_at <= ?", filter.DateTo+" 23:59:59")
	}

	// Source/domain filter
	if filter.Source != "" {
		query = query.Where("url LIKE ?", "%"+filter.Source+"%")
	}

	// Location filter
	if filter.Location != "" {
		query = query.Where("location LIKE ?", "%"+filter.Location+"%")
	}

	// Determine sort order
	sortField := "created_at"
	sortDir := "DESC"
	switch filter.SortBy {
	case "title":
		sortField = "title"
	case "updated_at":
		sortField = "updated_at"
	}
	if filter.SortOrder == "asc" {
		sortDir = "ASC"
	}

	// Count total
	var total int64
	query.Model(&model.CatalogEntry{}).Count(&total)

	// Apply pagination
	offset := (filter.Page - 1) * filter.Limit
	var entries []*model.CatalogEntry
	err := query.Order(sortField + " " + sortDir).
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

// Search performs full-text search on entries
func (r *EntryRepository) Search(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	searchPattern := "%" + query + "%"

	// Build base query
	baseQuery := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(phone_number) LIKE ? OR LOWER(location) LIKE ? OR LOWER(url) LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)

	// Apply ExcludeTried filter in SQL if requested
	if filter.ExcludeTried {
		baseQuery = baseQuery.Where("id NOT IN (SELECT entry_id FROM interactions WHERE user_id = ? AND tried = ?)", userID, true)
	}

	// Date range filter
	if filter.DateFrom != "" {
		baseQuery = baseQuery.Where("created_at >= ?", filter.DateFrom)
	}
	if filter.DateTo != "" {
		baseQuery = baseQuery.Where("created_at <= ?", filter.DateTo+" 23:59:59")
	}

	// Source/domain filter
	if filter.Source != "" {
		baseQuery = baseQuery.Where("url LIKE ?", "%"+filter.Source+"%")
	}

	// Location filter
	if filter.Location != "" {
		baseQuery = baseQuery.Where("location LIKE ?", "%"+filter.Location+"%")
	}

	// HasInteraction filter - only entries with interactions
	if filter.HasInteraction {
		baseQuery = baseQuery.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ?)", userID)
	}

	// MinScore filter - entries where user's score >= minScore (implies HasInteraction)
	if filter.MinScore > 0 {
		baseQuery = baseQuery.Where("id IN (SELECT entry_id FROM interactions WHERE user_id = ? AND score >= ?)", userID, filter.MinScore)
	}

	// Determine sort order
	sortField := "created_at"
	sortDir := "DESC"
	switch filter.SortBy {
	case "title":
		sortField = "title"
	case "updated_at":
		sortField = "updated_at"
	}
	if filter.SortOrder == "asc" {
		sortDir = "ASC"
	}

	// Count total matching entries
	var total int64
	baseQuery.Model(&model.CatalogEntry{}).Count(&total)

	// Apply pagination
	offset := (filter.Page - 1) * filter.Limit
	var entries []*model.CatalogEntry
	err := baseQuery.Order(sortField + " " + sortDir).
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
	return r.db.WithContext(ctx).Save(entry).Error
}

// Delete deletes an entry by ID (only if owned by user)
func (r *EntryRepository) Delete(ctx context.Context, id, userID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First verify ownership
		var entry model.CatalogEntry
		if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&entry).Error; err != nil {
			return err
		}

		// Get revision IDs for this entry
		var revisionIDs []string
		if err := tx.Model(&model.ArchiveRevision{}).Where("entry_id = ?", id).Pluck("id", &revisionIDs).Error; err != nil {
			return err
		}

		// Delete archive assets by revision IDs
		if len(revisionIDs) > 0 {
			if err := tx.Where("revision_id IN ?", revisionIDs).Delete(&model.ArchiveAsset{}).Error; err != nil {
				return err
			}
		}
		// Delete archive revisions
		if err := tx.Where("entry_id = ?", id).Delete(&model.ArchiveRevision{}).Error; err != nil {
			return err
		}
		// Delete entry tags
		if err := tx.Where("entry_id = ?", id).Delete(&model.EntryTag{}).Error; err != nil {
			return err
		}
		// Delete interactions
		if err := tx.Where("entry_id = ?", id).Delete(&model.Interaction{}).Error; err != nil {
			return err
		}
		// Delete entry
		return tx.Where("id = ?", id).Delete(&model.CatalogEntry{}).Error
	})
}

// GetByUserIDWithTags finds all entries for a user with their tags
func (r *EntryRepository) GetByUserIDWithTags(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
	result, err := r.GetByUserID(ctx, userID, filter)
	if err != nil {
		return nil, err
	}

	// Fetch tags for all entries (placeholder - extend model if needed)
	_ = r.db

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

// FindRandomEntry finds a random entry for a user
func (r *EntryRepository) FindRandomEntry(ctx context.Context, userID string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("RANDOM()").
		Take(&entry).Error
	return &entry, err
}

// FindRandomTriedEntry finds a random entry that hasn't been tried yet
func (r *EntryRepository) FindRandomTriedEntry(ctx context.Context, userID string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Table("catalog_entries").
		Select("catalog_entries.*").
		Joins("LEFT JOIN interactions ON catalog_entries.id = interactions.entry_id AND interactions.user_id = ?", userID).
		Where("catalog_entries.user_id = ?", userID).
		Where("interactions.id IS NULL OR interactions.tried = ?", false).
		Order("RANDOM()").
		Take(&entry).Error
	return &entry, err
}

// FindRandomEntryWithTag finds a random entry with a specific tag
func (r *EntryRepository) FindRandomEntryWithTag(ctx context.Context, userID, tagID string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Table("catalog_entries").
		Select("catalog_entries.*").
		Joins("JOIN entry_tags ON catalog_entries.id = entry_tags.entry_id").
		Where("catalog_entries.user_id = ?", userID).
		Where("entry_tags.tag_id = ?", tagID).
		Order("RANDOM()").
		Take(&entry).Error
	return &entry, err
}

// FindRandomTriedEntryWithTag finds a random entry with a specific tag that hasn't been tried
func (r *EntryRepository) FindRandomTriedEntryWithTag(ctx context.Context, userID, tagID string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Table("catalog_entries").
		Select("catalog_entries.*").
		Joins("JOIN entry_tags ON catalog_entries.id = entry_tags.entry_id").
		Joins("LEFT JOIN interactions ON catalog_entries.id = interactions.entry_id AND interactions.user_id = ?", userID).
		Where("catalog_entries.user_id = ?", userID).
		Where("entry_tags.tag_id = ?", tagID).
		Where("interactions.id IS NULL OR interactions.tried = ?", false).
		Order("RANDOM()").
		Take(&entry).Error
	return &entry, err
}

// FindByURL finds an entry by URL for a specific user
func (r *EntryRepository) FindByURL(ctx context.Context, userID, url string) (*model.CatalogEntry, error) {
	var entry model.CatalogEntry
	err := r.db.WithContext(ctx).Where("user_id = ? AND url = ?", userID, url).First(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// CountByUserID counts entries for a user
func (r *EntryRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.CatalogEntry{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// FindPendingForArchiving finds entries pending archival
func (r *EntryRepository) FindPendingForArchiving(ctx context.Context, limit int) ([]*model.CatalogEntry, error) {
	var entries []*model.CatalogEntry
	err := r.db.WithContext(ctx).
		Where("archive_status = ?", model.ArchiveStatusPending).
		Or("archive_status = ? AND archive_path = ?", model.ArchiveStatusFailed, "").
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
