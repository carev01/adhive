package repository

import (
	"context"

	"github.com/carev01/adhive/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetByURLs finds entries by URLs for a specific user, returning a map of URL -> entry
func (r *EntryRepository) GetByURLs(ctx context.Context, userID string, urls []string) (map[string]*model.CatalogEntry, error) {
	if len(urls) == 0 {
		return make(map[string]*model.CatalogEntry), nil
	}
	var entries []*model.CatalogEntry
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND url IN ?", userID, urls).
		Find(&entries).Error
	if err != nil {
		return nil, WrapDBError(err, "Entry", "")
	}
	result := make(map[string]*model.CatalogEntry, len(entries))
	for _, entry := range entries {
		result[entry.URL] = entry
	}
	return result, nil
}

// BatchCreate creates multiple entries in a single transaction
func (r *EntryRepository) BatchCreate(ctx context.Context, entries []*model.CatalogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.CreateInBatches(entries, 100).Error; err != nil {
			return WrapDBError(err, "Entry", "batch")
		}
		return nil
	})
}

// BatchUpdate updates multiple entries in a single transaction
func (r *EntryRepository) BatchUpdate(ctx context.Context, entries []*model.CatalogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, entry := range entries {
			result := tx.Model(&model.CatalogEntry{}).
				Where("id = ?", entry.ID).
				Updates(map[string]interface{}{
					"title":        entry.Title,
					"description":  entry.Description,
					"phone_number": entry.PhoneNumber,
					"location":     entry.Location,
					"updated_at":   entry.UpdatedAt,
				})
			if result.Error != nil {
				return WrapDBError(result.Error, "Entry", entry.ID)
			}
		}
		return nil
	})
}

// ExportByUserID exports all entries for a user, optionally filtered
func (r *EntryRepository) ExportByUserID(ctx context.Context, userID string, filter *model.EntryFilter) ([]*model.CatalogEntry, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if filter != nil {
		// Apply filters inline (matches the filter logic from GetByUserID)
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
		if filter.Search != "" {
			searchPattern := "%" + filter.Search + "%"
			query = query.Where(
				"LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(url) LIKE ?",
				searchPattern, searchPattern, searchPattern,
			)
		}
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

	var entries []*model.CatalogEntry
	err := query.Order(sortField + " " + sortDir).Find(&entries).Error
	if err != nil {
		return nil, WrapDBError(err, "Entry", "")
	}
	return entries, nil
}

// BatchUpsertByURL creates new entries and updates existing ones in a batch.
func (r *EntryRepository) BatchUpsertByURL(ctx context.Context, userID string, creates []*model.CatalogEntry, updates map[string]*model.CatalogEntry) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(creates) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).CreateInBatches(creates, 100).Error; err != nil {
				return WrapDBError(err, "Entry", "batch-create")
			}
		}
		for _, entry := range updates {
			result := tx.Model(&model.CatalogEntry{}).
				Where("id = ? AND user_id = ?", entry.ID, userID).
				Updates(map[string]interface{}{
					"title":        entry.Title,
					"description":  entry.Description,
					"phone_number": entry.PhoneNumber,
					"location":     entry.Location,
					"updated_at":   entry.UpdatedAt,
				})
			if result.Error != nil {
				return WrapDBError(result.Error, "Entry", entry.ID)
			}
		}
		return nil
	})
}