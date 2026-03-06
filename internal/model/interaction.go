package model

import (
	"time"
)

// Interaction represents a user's interaction with a catalog entry
type Interaction struct {
	ID           string     `json:"id" gorm:"primaryKey;type:text"`
	EntryID      string     `json:"entry_id" gorm:"index;not null"`
	UserID       string     `json:"user_id" gorm:"index;not null"`
	Tried        bool       `json:"tried" gorm:"default:false"`
	Score        *int       `json:"score" gorm:"type:integer"` // 0-5, nullable
	Comments     string     `json:"comments" gorm:"type:text"`
	ContactedAt  *time.Time `json:"contacted_at"`
	PurchasedAt  *time.Time `json:"purchased_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (Interaction) TableName() string {
	return "interactions"
}

// InteractionInput represents the input for creating/updating an interaction
type InteractionInput struct {
	Tried       *bool       `json:"tried"`
	Score       *int        `json:"score"`   // 0-5
	Comments    *string     `json:"comments"`
	ContactedAt *time.Time  `json:"contacted_at"`
	PurchasedAt *time.Time  `json:"purchased_at"`
}

// Validate validates the interaction input
func (i *InteractionInput) Validate() bool {
	if i.Score != nil && (*i.Score < 0 || *i.Score > 5) {
		return false
	}
	return true
}

// SiteMetadata represents site-specific extracted metadata
type SiteMetadata struct {
	Source      string   `json:"source,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Price       string   `json:"price,omitempty"`
	Location    string   `json:"location,omitempty"`
	Phones      []string `json:"phones,omitempty"`
	Images      []string `json:"images,omitempty"`
	Category    string   `json:"category,omitempty"`
	Seller      string   `json:"seller,omitempty"`
}