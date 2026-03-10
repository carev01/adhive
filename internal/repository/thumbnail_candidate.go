package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/carev01/adhive/internal/model"
)

// ThumbnailCandidateRepository handles thumbnail candidate persistence.
type ThumbnailCandidateRepository struct {
	db *gorm.DB
}

func NewThumbnailCandidateRepository(db *gorm.DB) *ThumbnailCandidateRepository {
	return &ThumbnailCandidateRepository{db: db}
}

func (r *ThumbnailCandidateRepository) Upsert(ctx context.Context, candidate *model.ThumbnailCandidate) error {
	return r.db.WithContext(ctx).
		Where("id = ?", candidate.ID).
		Assign(candidate).
		FirstOrCreate(candidate).Error
}

func (r *ThumbnailCandidateRepository) Create(ctx context.Context, candidate *model.ThumbnailCandidate) error {
	return r.db.WithContext(ctx).Create(candidate).Error
}

func (r *ThumbnailCandidateRepository) ListByEntryID(ctx context.Context, entryID string) ([]*model.ThumbnailCandidate, error) {
	var candidates []*model.ThumbnailCandidate
	err := r.db.WithContext(ctx).
		Where("entry_id = ?", entryID).
		Order("selected DESC, score DESC, created_at DESC").
		Find(&candidates).Error
	return candidates, err
}

func (r *ThumbnailCandidateRepository) GetByID(ctx context.Context, id string) (*model.ThumbnailCandidate, error) {
	var candidate model.ThumbnailCandidate
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&candidate).Error
	if err != nil {
		return nil, err
	}
	return &candidate, nil
}

func (r *ThumbnailCandidateRepository) ClearSelected(ctx context.Context, entryID string) error {
	return r.db.WithContext(ctx).
		Model(&model.ThumbnailCandidate{}).
		Where("entry_id = ?", entryID).
		Update("selected", false).Error
}

func (r *ThumbnailCandidateRepository) Select(ctx context.Context, entryID, candidateID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ThumbnailCandidate{}).
			Where("entry_id = ?", entryID).
			Update("selected", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ThumbnailCandidate{}).
			Where("entry_id = ? AND id = ?", entryID, candidateID).
			Update("selected", true).Error; err != nil {
			return err
		}
		return nil
	})
}
