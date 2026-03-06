package repository

import (
	"context"

	"github.com/carev01/adhive/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InteractionRepository handles interaction database operations
type InteractionRepository struct {
	db *gorm.DB
}

// NewInteractionRepository creates a new InteractionRepository
func NewInteractionRepository(db *gorm.DB) *InteractionRepository {
	return &InteractionRepository{db: db}
}

// GetByEntryAndUser gets an interaction by entry and user
func (r *InteractionRepository) GetByEntryAndUser(ctx context.Context, entryID, userID string) (*model.Interaction, error) {
	var interaction model.Interaction
	err := r.db.WithContext(ctx).
		Where("entry_id = ? AND user_id = ?", entryID, userID).
		First(&interaction).Error
	if err != nil {
		return nil, err
	}
	return &interaction, nil
}

// Create creates a new interaction
func (r *InteractionRepository) Create(ctx context.Context, interaction *model.Interaction) error {
	return r.db.WithContext(ctx).Create(interaction).Error
}

// Update updates an existing interaction
func (r *InteractionRepository) Update(ctx context.Context, interaction *model.Interaction) error {
	return r.db.WithContext(ctx).Save(interaction).Error
}

// Upsert creates or updates an interaction for an entry/user
// Note: Any interaction automatically means the entry was "tried"
func (r *InteractionRepository) Upsert(ctx context.Context, entryID, userID string, input *model.InteractionInput) (*model.Interaction, error) {
	var interaction model.Interaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Try to find existing
		err := tx.Where("entry_id = ? AND user_id = ?", entryID, userID).First(&interaction).Error
		if err == gorm.ErrRecordNotFound {
			// Create new - any interaction means tried = true
			interaction = model.Interaction{
				ID:      uuid.New().String(),
				EntryID: entryID,
				UserID:  userID,
				Tried:   true, // Any interaction means the entry was tried
			}
		} else if err != nil {
			return err
		}

		// Apply updates (tried stays true once an interaction exists)
		// Score, comments, dates can be updated
		if input.Score != nil {
			interaction.Score = input.Score
		}
		if input.Comments != nil {
			interaction.Comments = *input.Comments
		}
		if input.ContactedAt != nil {
			interaction.ContactedAt = input.ContactedAt
		}
		if input.PurchasedAt != nil {
			interaction.PurchasedAt = input.PurchasedAt
		}

		// Ensure tried is always true for any interaction
		interaction.Tried = true

		// Save (create or update)
		return tx.Save(&interaction).Error
	})

	if err != nil {
		return nil, err
	}
	return &interaction, nil
}

// Delete deletes an interaction
func (r *InteractionRepository) Delete(ctx context.Context, entryID, userID string) error {
	return r.db.WithContext(ctx).
		Where("entry_id = ? AND user_id = ?", entryID, userID).
		Delete(&model.Interaction{}).Error
}

// GetTriedEntryIDs gets all entry IDs that have been marked as tried
func (r *InteractionRepository) GetTriedEntryIDs(ctx context.Context, userID string) ([]string, error) {
	var entryIDs []string
	err := r.db.WithContext(ctx).
		Model(&model.Interaction{}).
		Where("user_id = ? AND tried = ?", userID, true).
		Pluck("entry_id", &entryIDs).Error
	return entryIDs, err
}