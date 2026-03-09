package model

import (
	"encoding/json"
	"time"
)

// ArchiveStatus represents the status of an entry's archival process
type ArchiveStatus string

const (
	ArchiveStatusPending ArchiveStatus = "pending"
	ArchiveStatusSuccess ArchiveStatus = "success"
	ArchiveStatusFailed  ArchiveStatus = "failed"
)

// CatalogEntry represents a catalog entry in the system (matches handler expectations)
type CatalogEntry struct {
	ID            string          `json:"id" gorm:"primaryKey;type:text"`
	UserID        string          `json:"user_id" gorm:"index;not null"`
	URL           string          `json:"url" gorm:"type:text;not null"`
	Title         string          `json:"title" gorm:"type:varchar(500)"`
	Description   string          `json:"description" gorm:"type:text"`
	PhoneNumber   string          `json:"phone_number" gorm:"type:varchar(50)"`
	Location      string          `json:"location" gorm:"type:varchar(255)"`
	ThumbnailPath string          `json:"thumbnail_path" gorm:"type:varchar(500)"`
	ArchivePath   string          `json:"archive_path" gorm:"type:varchar(500)"`
	ArchiveStatus             ArchiveStatus    `json:"archive_status" gorm:"type:varchar(20);default:pending"`
	ArchiveCurrentRevisionID  *string          `json:"archive_current_revision_id,omitempty" gorm:"type:text;index"`
	ArchiveFidelity           ArchiveFidelity  `json:"archive_fidelity" gorm:"type:text;default:low"`
	ThumbnailSource           ThumbnailSource  `json:"thumbnail_source" gorm:"type:text;default:auto"`
	MetadataRaw               json.RawMessage  `json:"metadata_raw" gorm:"type:text"`
	CreatedAt                 time.Time        `json:"created_at"`
	UpdatedAt                 time.Time        `json:"updated_at"`
	LastCheckedAt             *time.Time       `json:"last_checked_at"`
	ImportedFrom              string           `json:"imported_from" gorm:"type:varchar(50"`
}

// TableName specifies the table name for GORM
func (CatalogEntry) TableName() string {
	return "catalog_entries"
}

// EntryCreateInput represents the input for creating a new entry
type EntryCreateInput struct {
	URL         string          `json:"url" binding:"required,url"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	PhoneNumber string          `json:"phone_number"`
	Location    string          `json:"location"`
	Metadata    json.RawMessage `json:"metadata"`
}

// EntryUpdateInput represents the input for updating an entry
type EntryUpdateInput struct {
	Title         *string        `json:"title"`
	Description   *string        `json:"description"`
	PhoneNumber   *string        `json:"phone_number"`
	Location      *string        `json:"location"`
	ThumbnailPath *string        `json:"thumbnail_path"`
	ArchivePath   *string        `json:"archive_path"`
	ArchiveStatus *ArchiveStatus `json:"archive_status"`
	Metadata      json.RawMessage `json:"metadata"`
}

// EntryFilter represents filter options for querying entries (matches handler expectations)
type EntryFilter struct {
	Page           int
	Limit          int
	TagID          string
	Search         string
	Status         ArchiveStatus
	ExcludeTried   bool
	DateFrom       string
	DateTo         string
	Source         string
	Location       string // Filter by location
	SortBy         string
	SortOrder      string
	HasInteraction bool   // Filter to only entries with interactions
	MinScore       int    // Minimum score filter (0-5)
}

type EntryListResult struct {
	Entries []*CatalogEntry
	Total   int64
	Page    int
	Limit   int
}
