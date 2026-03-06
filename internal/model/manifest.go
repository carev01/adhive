package model

import "time"

// ArchiveManifest is the canonical metadata document for a revision bundle.
type ArchiveManifest struct {
	SchemaVersion string                 `json:"schema_version"`
	RevisionID    string                 `json:"revision_id"`
	EntryID       string                 `json:"entry_id"`
	RevisionNo    int                    `json:"revision_no"`
	CapturedAt    time.Time              `json:"captured_at"`
	Engine        ArchiveEngine          `json:"engine"`
	BaseURL       string                 `json:"base_url"`
	Status        ArchiveRevisionStatus  `json:"status"`
	FailureReason string                 `json:"failure_reason,omitempty"`
	Stats         ArchiveManifestStats   `json:"stats"`
	Diagnostics   ArchiveManifestDiag    `json:"diagnostics"`
	Files         []ArchiveManifestFile  `json:"files"`
	Rewrites      []ArchiveManifestRewrite `json:"rewrites,omitempty"`
}

// ArchiveManifestStats aggregates archive sizing and counts.
type ArchiveManifestStats struct {
	TotalAssets      int   `json:"total_assets"`
	DownloadedAssets int   `json:"downloaded_assets"`
	MissingAssets    int   `json:"missing_assets"`
	SkippedAssets    int   `json:"skipped_assets"`
	ErrorAssets      int   `json:"error_assets"`
	TotalBytes       int64 `json:"total_bytes"`
}

// ArchiveManifestDiag records capture diagnostics useful for reliability tuning.
type ArchiveManifestDiag struct {
	FinalURL          string   `json:"final_url,omitempty"`
	HTTPStatus        int      `json:"http_status,omitempty"`
	RedirectChain     []string `json:"redirect_chain,omitempty"`
	ChallengeDetected bool     `json:"challenge_detected,omitempty"`
	TimeoutStage      string   `json:"timeout_stage,omitempty"`
	ErrorType         string   `json:"error_type,omitempty"`
}

// ArchiveManifestFile maps original source metadata to local bundled file path.
type ArchiveManifestFile struct {
	SourceURL      string                     `json:"source_url"`
	LocalPath      string                     `json:"local_path"`
	ContentHash    string                     `json:"content_hash,omitempty"`
	MimeType       string                     `json:"mime_type,omitempty"`
	Bytes          int64                      `json:"bytes"`
	Kind           ArchiveAssetKind           `json:"kind"`
	DownloadStatus ArchiveAssetDownloadStatus `json:"download_status"`
}

// ArchiveManifestRewrite stores rewritten reference mappings.
type ArchiveManifestRewrite struct {
	From string `json:"from"`
	To   string `json:"to"`
}
