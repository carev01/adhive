package model

import "time"

// ArchiveEngine identifies the capture engine used for a revision.
type ArchiveEngine string

const (
	ArchiveEnginePlaywright ArchiveEngine = "playwright"
	ArchiveEngineHTTP       ArchiveEngine = "http"
)

// ArchiveRevisionStatus indicates archive quality/completion for a revision.
type ArchiveRevisionStatus string

const (
	ArchiveRevisionStatusSuccess ArchiveRevisionStatus = "success"
	ArchiveRevisionStatusPartial ArchiveRevisionStatus = "partial"
	ArchiveRevisionStatusFailed  ArchiveRevisionStatus = "failed"
	ArchiveRevisionStatusBlocked ArchiveRevisionStatus = "blocked"
)

// ArchiveAssetKind classifies archived asset type.
type ArchiveAssetKind string

const (
	ArchiveAssetKindCSS   ArchiveAssetKind = "css"
	ArchiveAssetKindJS    ArchiveAssetKind = "js"
	ArchiveAssetKindIMG   ArchiveAssetKind = "img"
	ArchiveAssetKindFont  ArchiveAssetKind = "font"
	ArchiveAssetKindMedia ArchiveAssetKind = "media"
	ArchiveAssetKindOther ArchiveAssetKind = "other"
)

// ArchiveAssetDownloadStatus indicates per-asset fetch outcome.
type ArchiveAssetDownloadStatus string

const (
	ArchiveAssetDownloadStatusOK      ArchiveAssetDownloadStatus = "ok"
	ArchiveAssetDownloadStatusMissing ArchiveAssetDownloadStatus = "missing"
	ArchiveAssetDownloadStatusSkipped ArchiveAssetDownloadStatus = "skipped"
	ArchiveAssetDownloadStatusError   ArchiveAssetDownloadStatus = "error"
)

// ArchiveFidelity summarizes the quality of the current archive for an entry.
type ArchiveFidelity string

const (
	ArchiveFidelityHigh    ArchiveFidelity = "high"
	ArchiveFidelityPartial ArchiveFidelity = "partial"
	ArchiveFidelityLow     ArchiveFidelity = "low"
)

// ThumbnailSource indicates how the thumbnail currently in use was chosen.
type ThumbnailSource string

const (
	ThumbnailSourceAuto         ThumbnailSource = "auto"
	ThumbnailSourceUserSelected ThumbnailSource = "user_selected"
	ThumbnailSourceUpload       ThumbnailSource = "upload"
)

// ThumbnailCandidateSourceType classifies where a thumbnail candidate came from.
type ThumbnailCandidateSourceType string

const (
	ThumbnailCandidateSourceLocalAsset ThumbnailCandidateSourceType = "local_asset"
	ThumbnailCandidateSourceScreenshot ThumbnailCandidateSourceType = "screenshot"
	ThumbnailCandidateSourceRemoteMeta ThumbnailCandidateSourceType = "remote_meta"
	ThumbnailCandidateSourceUpload     ThumbnailCandidateSourceType = "upload"
	ThumbnailCandidateSourceArchive    ThumbnailCandidateSourceType = "archive"
)

// ArchiveRevision tracks each archive capture for an entry.
type ArchiveRevision struct {
	ID            string                `json:"id" gorm:"primaryKey;type:text"`
	EntryID       string                `json:"entry_id" gorm:"type:text;not null;index"`
	RevisionNo    int                   `json:"revision_no" gorm:"not null"`
	Engine        ArchiveEngine         `json:"engine" gorm:"type:text;not null"`
	RootPath      string                `json:"root_path" gorm:"type:text;not null"`
	IndexPath     string                `json:"index_path" gorm:"type:text;not null"`
	ManifestPath  string                `json:"manifest_path" gorm:"type:text;not null"`
	Status        ArchiveRevisionStatus `json:"status" gorm:"type:text;not null"`
	FailureReason *string               `json:"failure_reason,omitempty" gorm:"type:text"`
	CapturedAt    time.Time             `json:"captured_at"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

func (ArchiveRevision) TableName() string {
	return "archive_revisions"
}

// ArchiveAsset stores metadata for each archived asset in a revision.
type ArchiveAsset struct {
	ID             string                     `json:"id" gorm:"primaryKey;type:text"`
	RevisionID     string                     `json:"revision_id" gorm:"type:text;not null;index"`
	RootPath       string                     `json:"root_path" gorm:"type:text"` // Populated from revision's root_path
	SourceURL      string                     `json:"source_url" gorm:"type:text;not null"`
	LocalPath      string                     `json:"local_path" gorm:"type:text;not null"`
	ContentHash    string                     `json:"content_hash,omitempty" gorm:"type:text;index"`
	MimeType       string                     `json:"mime_type,omitempty" gorm:"type:text"`
	Bytes          int64                      `json:"bytes" gorm:"not null;default:0"`
	Kind           ArchiveAssetKind           `json:"kind" gorm:"type:text;not null;index"`
	DownloadStatus ArchiveAssetDownloadStatus `json:"download_status" gorm:"type:text;not null"`
	CreatedAt      time.Time                  `json:"created_at"`
}

func (ArchiveAsset) TableName() string {
	return "archive_assets"
}

// ThumbnailCandidate is a potential thumbnail option extracted during archive flows.
type ThumbnailCandidate struct {
	ID         string                       `json:"id" gorm:"primaryKey;type:text"`
	EntryID    string                       `json:"entry_id" gorm:"type:text;not null;index"`
	RevisionID *string                      `json:"revision_id,omitempty" gorm:"type:text;index"`
	SourceType ThumbnailCandidateSourceType `json:"source_type" gorm:"type:text;not null"`
	Path       string                       `json:"path" gorm:"type:text;not null"`
	Score      float64                      `json:"score" gorm:"not null;default:0"`
	Selected   bool                         `json:"selected" gorm:"not null;default:false;index"`
	CreatedAt  time.Time                    `json:"created_at"`
}

func (ThumbnailCandidate) TableName() string {
	return "thumbnail_candidates"
}
