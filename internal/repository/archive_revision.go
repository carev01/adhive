package repository

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"

	"github.com/carev01/adhive/internal/model"
)

// ArchiveRevisionRepository handles archive_revisions persistence.
type ArchiveRevisionRepository struct {
	db *gorm.DB
}

func NewArchiveRevisionRepository(db *gorm.DB) *ArchiveRevisionRepository {
	return &ArchiveRevisionRepository{db: db}
}

func (r *ArchiveRevisionRepository) Create(ctx context.Context, revision *model.ArchiveRevision) error {
	return r.db.WithContext(ctx).Create(revision).Error
}

func (r *ArchiveRevisionRepository) GetByID(ctx context.Context, id string) (*model.ArchiveRevision, error) {
	var revision model.ArchiveRevision
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&revision).Error
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func (r *ArchiveRevisionRepository) NextRevisionNo(ctx context.Context, entryID string) (int, error) {
	var next sql.NullInt64
	err := r.db.WithContext(ctx).
		Raw("SELECT COALESCE(MAX(revision_no), 0) + 1 FROM archive_revisions WHERE entry_id = ?", entryID).
		Scan(&next).Error
	if err != nil {
		return 0, err
	}
	if !next.Valid {
		return 1, nil
	}
	return int(next.Int64), nil
}

func (r *ArchiveRevisionRepository) ListByEntryID(ctx context.Context, entryID string) ([]*model.ArchiveRevision, error) {
	var revisions []*model.ArchiveRevision
	err := r.db.WithContext(ctx).
		Where("entry_id = ?", entryID).
		Order("revision_no DESC").
		Find(&revisions).Error
	return revisions, err
}

func (r *ArchiveRevisionRepository) DeleteByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.ArchiveRevision{}).Error
}

// ArchiveMetrics is an aggregated operational view for archive quality.
type ArchiveMetrics struct {
	TotalRevisions int64 `json:"total_revisions"`
	SuccessCount   int64 `json:"success_count"`
	PartialCount   int64 `json:"partial_count"`
	BlockedCount   int64 `json:"blocked_count"`
	FailedCount    int64 `json:"failed_count"`
	TotalAssets    int64 `json:"total_assets"`
	TotalBytes     int64 `json:"total_bytes"`
}

func (r *ArchiveRevisionRepository) Metrics(ctx context.Context, since time.Time) (*ArchiveMetrics, error) {
	m := &ArchiveMetrics{}
	q := r.db.WithContext(ctx).Model(&model.ArchiveRevision{})
	if !since.IsZero() {
		q = q.Where("captured_at >= ?", since)
	}
	if err := q.Count(&m.TotalRevisions).Error; err != nil {
		return nil, err
	}
	if err := q.Where("status = ?", model.ArchiveRevisionStatusSuccess).Count(&m.SuccessCount).Error; err != nil {
		return nil, err
	}
	if err := q.Where("status = ?", model.ArchiveRevisionStatusPartial).Count(&m.PartialCount).Error; err != nil {
		return nil, err
	}
	if err := q.Where("status = ?", model.ArchiveRevisionStatusBlocked).Count(&m.BlockedCount).Error; err != nil {
		return nil, err
	}
	if err := q.Where("status = ?", model.ArchiveRevisionStatusFailed).Count(&m.FailedCount).Error; err != nil {
		return nil, err
	}

	assetQ := r.db.WithContext(ctx).Model(&model.ArchiveAsset{})
	if !since.IsZero() {
		assetQ = assetQ.Where("created_at >= ?", since)
	}
	if err := assetQ.Count(&m.TotalAssets).Error; err != nil {
		return nil, err
	}
	if err := assetQ.Select("COALESCE(SUM(bytes),0)").Scan(&m.TotalBytes).Error; err != nil {
		return nil, err
	}
	return m, nil
}
