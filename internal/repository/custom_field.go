package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/carev01/adhive/internal/model"
)

// CustomFieldRepository handles custom field database operations
type CustomFieldRepository struct {
	db *gorm.DB
}

// NewCustomFieldRepository creates a new CustomFieldRepository
func NewCustomFieldRepository(db *gorm.DB) *CustomFieldRepository {
	return &CustomFieldRepository{db: db}
}

// Create creates a new custom field
func (r *CustomFieldRepository) Create(ctx context.Context, cf *model.CustomField) error {
	err := r.db.WithContext(ctx).Create(cf).Error
	if err != nil {
		return WrapDBError(err, "CustomField", cf.ID)
	}
	return nil
}

// GetByID finds a custom field by ID
func (r *CustomFieldRepository) GetByID(ctx context.Context, id string) (*model.CustomField, error) {
	var cf model.CustomField
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&cf).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", id)
	}
	return &cf, nil
}

// GetByEntryID returns all custom fields for an entry
func (r *CustomFieldRepository) GetByEntryID(ctx context.Context, entryID string) ([]model.CustomField, error) {
	var fields []model.CustomField
	err := r.db.WithContext(ctx).Where("entry_id = ?", entryID).Order("field_name ASC").Find(&fields).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", "")
	}
	return fields, nil
}

// GetByEntryIDAndName finds a custom field by entry ID and field name
func (r *CustomFieldRepository) GetByEntryIDAndName(ctx context.Context, entryID, fieldName string) (*model.CustomField, error) {
	var cf model.CustomField
	err := r.db.WithContext(ctx).Where("entry_id = ? AND field_name = ?", entryID, fieldName).First(&cf).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", "")
	}
	return &cf, nil
}

// Update updates a custom field
func (r *CustomFieldRepository) Update(ctx context.Context, cf *model.CustomField) error {
	err := r.db.WithContext(ctx).Save(cf).Error
	if err != nil {
		return WrapDBError(err, "CustomField", cf.ID)
	}
	return nil
}

// Delete deletes a custom field by ID
func (r *CustomFieldRepository) Delete(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.CustomField{}).Error
	if err != nil {
		return WrapDBError(err, "CustomField", id)
	}
	return nil
}

// DeleteByEntryID deletes all custom fields for an entry
func (r *CustomFieldRepository) DeleteByEntryID(ctx context.Context, entryID string) error {
	err := r.db.WithContext(ctx).Where("entry_id = ?", entryID).Delete(&model.CustomField{}).Error
	if err != nil {
		return WrapDBError(err, "CustomField", "")
	}
	return nil
}

// BatchGetByEntryIDs fetches custom fields for multiple entries in a single query
func (r *CustomFieldRepository) BatchGetByEntryIDs(ctx context.Context, entryIDs []string) (map[string][]model.CustomField, error) {
	if len(entryIDs) == 0 {
		return map[string][]model.CustomField{}, nil
	}

	var fields []model.CustomField
	err := r.db.WithContext(ctx).Where("entry_id IN ?", entryIDs).Order("field_name ASC").Find(&fields).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", "")
	}

	result := make(map[string][]model.CustomField, len(entryIDs))
	for _, f := range fields {
		result[f.EntryID] = append(result[f.EntryID], f)
	}
	return result, nil
}

// GetDistinctFieldNames returns all distinct field names for a user's entries
func (r *CustomFieldRepository) GetDistinctFieldNames(ctx context.Context, userID string) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).
		Table("custom_fields").
		Select("DISTINCT custom_fields.field_name").
		Joins("JOIN catalog_entries ON catalog_entries.id = custom_fields.entry_id").
		Where("catalog_entries.user_id = ?", userID).
		Order("custom_fields.field_name ASC").
		Pluck("field_name", &names).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", "")
	}
	return names, nil
}

// GetFieldValueDistribution returns value counts for a specific field name for a user's entries
func (r *CustomFieldRepository) GetFieldValueDistribution(ctx context.Context, userID, fieldName string, limit int) ([]model.ValueCount, error) {
	var results []model.ValueCount
	query := r.db.WithContext(ctx).
		Table("custom_fields").
		Select("custom_fields.field_value as value, COUNT(*) as count").
		Joins("JOIN catalog_entries ON catalog_entries.id = custom_fields.entry_id").
		Where("catalog_entries.user_id = ? AND custom_fields.field_name = ?", userID, fieldName).
		Group("custom_fields.field_value").
		Order("count DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&results).Error
	if err != nil {
		return nil, WrapDBError(err, "CustomField", "")
	}
	return results, nil
}