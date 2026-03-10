package repository

import (
	"github.com/carev01/adhive/internal/model"
	"gorm.io/gorm"
)

// TagRepository handles tag database operations
type TagRepository struct {
	db *gorm.DB
}

// NewTagRepository creates a new TagRepository
func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{db: db}
}

// Create creates a new tag
func (r *TagRepository) Create(tag *model.Tag) error {
	err := r.db.Create(tag).Error
	return WrapDBError(err, "Tag", tag.ID)
}

// FindByID finds a tag by ID
func (r *TagRepository) FindByID(id string) (*model.Tag, error) {
	var tag model.Tag
	err := r.db.Where("id = ?", id).First(&tag).Error
	return &tag, WrapDBError(err, "Tag", id)
}

// FindByUserID finds all tags for a user
func (r *TagRepository) FindByUserID(userID string) ([]model.Tag, error) {
	var tags []model.Tag
	err := r.db.Where("user_id = ?", userID).Order("name ASC").Find(&tags).Error
	return tags, WrapDBError(err, "Tag", "")
}

// FindByName finds a tag by name for a user
func (r *TagRepository) FindByName(userID, name string) (*model.Tag, error) {
	var tag model.Tag
	err := r.db.Where("user_id = ? AND name = ?", userID, name).First(&tag).Error
	return &tag, WrapDBError(err, "Tag", "")
}

// Update updates a tag
func (r *TagRepository) Update(tag *model.Tag) error {
	err := r.db.Save(tag).Error
	return WrapDBError(err, "Tag", tag.ID)
}

// Delete deletes a tag by ID
func (r *TagRepository) Delete(id string) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Delete entry tags first
		if err := tx.Where("tag_id = ?", id).Delete(&model.EntryTag{}).Error; err != nil {
			return err
		}
		// Delete tag
		return tx.Where("id = ?", id).Delete(&model.Tag{}).Error
	})
	return WrapDBError(err, "Tag", id)
}

// CountByUserID counts tags for a user
func (r *TagRepository) CountByUserID(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.Tag{}).Where("user_id = ?", userID).Count(&count).Error
	return count, WrapDBError(err, "Tag", "")
}

// AddEntryTag associates a tag with an entry (idempotent - ignores if already exists)
func (r *TagRepository) AddEntryTag(entryID, tagID string) error {
	entryTag := model.EntryTag{
		EntryID: entryID,
		TagID:   tagID,
	}
	// Use FirstOrCreate to avoid UNIQUE constraint errors
	result := r.db.Where("entry_id = ? AND tag_id = ?", entryID, tagID).FirstOrCreate(&entryTag)
	return WrapDBError(result.Error, "EntryTag", "")
}

// RemoveEntryTag removes a tag from an entry
func (r *TagRepository) RemoveEntryTag(entryID, tagID string) error {
	err := r.db.Where("entry_id = ? AND tag_id = ?", entryID, tagID).Delete(&model.EntryTag{}).Error
	return WrapDBError(err, "EntryTag", "")
}

// GetEntryTags gets all tags for an entry
func (r *TagRepository) GetEntryTags(entryID string) ([]model.Tag, error) {
	var tags []model.Tag
	err := r.db.Table("tags").
		Joins("JOIN entry_tags ON tags.id = entry_tags.tag_id").
		Where("entry_tags.entry_id = ?", entryID).
		Find(&tags).Error
	return tags, WrapDBError(err, "Tag", "")
}

// GetEntryCount gets the number of entries with a specific tag
func (r *TagRepository) GetEntryCount(tagID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.EntryTag{}).Where("tag_id = ?", tagID).Count(&count).Error
	return count, WrapDBError(err, "Tag", tagID)
}

// GetTagsWithCount gets all tags for a user with entry counts
func (r *TagRepository) GetTagsWithCount(userID string) ([]model.TagWithCount, error) {
	var tags []model.Tag
	err := r.db.Where("user_id = ?", userID).Order("name ASC").Find(&tags).Error
	if err != nil {
		return nil, WrapDBError(err, "Tag", "")
	}

	result := make([]model.TagWithCount, len(tags))
	for i, tag := range tags {
		result[i] = model.TagWithCount{
			Tag:   tag,
			Count: 0, // Will be populated below
		}
		var count int64
		r.db.Model(&model.EntryTag{}).Where("tag_id = ?", tag.ID).Count(&count)
		result[i].Count = count
	}

	return result, nil
}
