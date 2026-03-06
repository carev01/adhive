package model

import (
	"time"
)

// Tag represents a tag for categorizing entries
type Tag struct {
	ID        string    `json:"id" gorm:"primaryKey;type:text"`
	UserID    string    `json:"user_id" gorm:"index;not null"`
	Name      string    `json:"name" gorm:"type:varchar(50);not null"`
	Color     string    `json:"color" gorm:"type:varchar(7);default:#6B7280"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName specifies the table name for GORM
func (Tag) TableName() string {
	return "tags"
}

// EntryTag represents the junction table between entries and tags
type EntryTag struct {
	EntryID string `json:"entry_id" gorm:"primaryKey;type:text"`
	TagID   string `json:"tag_id" gorm:"primaryKey;type:text"`
}

// TableName specifies the table name for GORM
func (EntryTag) TableName() string {
	return "entry_tags"
}

// TagCreateInput represents the input for creating a new tag
type TagCreateInput struct {
	Name  string `json:"name" binding:"required,max=50"`
	Color string `json:"color"`
}

// TagFilter represents filter options for querying tags
type TagFilter struct {
	UserID string
	Name   string
}

// TagWithCount represents a tag with its entry count
type TagWithCount struct {
	Tag   Tag   `json:"tag"`
	Count int64 `json:"count"`
}
