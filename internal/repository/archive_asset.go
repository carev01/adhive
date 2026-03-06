package repository

import (
	"context"

	"github.com/carev01/adhive/internal/model"
	"gorm.io/gorm"
)

// ArchiveAssetRepository handles archive_assets persistence.
type ArchiveAssetRepository struct {
	db *gorm.DB
}

func NewArchiveAssetRepository(db *gorm.DB) *ArchiveAssetRepository {
	return &ArchiveAssetRepository{db: db}
}

func (r *ArchiveAssetRepository) CreateBatch(ctx context.Context, assets []*model.ArchiveAsset) error {
	if len(assets) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&assets).Error
}

func (r *ArchiveAssetRepository) ListImageAssetsByEntry(ctx context.Context, entryID string) ([]*model.ArchiveAsset, error) {
	var assets []*model.ArchiveAsset
	err := r.db.WithContext(ctx).
		Table("archive_assets").
		Select("archive_assets.*, archive_revisions.root_path").
		Joins("JOIN archive_revisions ON archive_revisions.id = archive_assets.revision_id").
		Where("archive_revisions.entry_id = ?", entryID).
		Where("archive_assets.kind = ?", model.ArchiveAssetKindIMG).
		Where("archive_assets.download_status = ?", model.ArchiveAssetDownloadStatusOK).
		Order("archive_revisions.revision_no DESC, archive_assets.created_at ASC").
		Find(&assets).Error
	return assets, err
}

func (r *ArchiveAssetRepository) DeleteByRevisionID(ctx context.Context, revisionID string) error {
	return r.db.WithContext(ctx).Where("revision_id = ?", revisionID).Delete(&model.ArchiveAsset{}).Error
}

func (r *ArchiveAssetRepository) GetByRevisionID(ctx context.Context, revisionID string) ([]*model.ArchiveAsset, error) {
	var assets []*model.ArchiveAsset
	err := r.db.WithContext(ctx).
		Where("revision_id = ?", revisionID).
		Order("bytes DESC, created_at ASC").
		Find(&assets).Error
	return assets, err
}
