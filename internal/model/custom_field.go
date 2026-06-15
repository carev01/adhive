package model

import (
	"time"
)

// CustomField represents a custom key-value field on a catalog entry (EAV pattern)
type CustomField struct {
	ID         string    `json:"id" gorm:"primaryKey;type:text"`
	EntryID    string    `json:"entry_id" gorm:"index:idx_custom_fields_entry_field;not null;type:text"`
	FieldName  string    `json:"field_name" gorm:"type:varchar(100);not null;index:idx_custom_fields_entry_field"`
	FieldValue string    `json:"field_value" gorm:"type:text;not null;default:''"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (CustomField) TableName() string {
	return "custom_fields"
}

// CustomFieldInput represents the input for creating/updating a custom field
type CustomFieldInput struct {
	FieldName  string `json:"field_name" binding:"required,max=100"`
	FieldValue string `json:"field_value"`
}

// CustomFieldUpdateInput represents the input for updating a custom field
type CustomFieldUpdateInput struct {
	FieldValue string `json:"field_value" binding:"required"`
}

// CustomFieldResponse represents a custom field in API responses
type CustomFieldResponse struct {
	ID         string `json:"id"`
	EntryID    string `json:"entry_id"`
	FieldName  string `json:"field_name"`
	FieldValue string `json:"field_value"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// FacetResult holds aggregate filter options for the facets endpoint
type FacetResult struct {
	Tags         []TagFacet         `json:"tags"`
	Statuses     []StatusFacet      `json:"statuses"`
	CustomFields []CustomFieldFacet `json:"custom_fields"`
	DateRange    *DateRangeFacet    `json:"date_range"`
}

// TagFacet represents a tag with its entry count for faceted filtering
type TagFacet struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int64  `json:"count"`
}

// StatusFacet represents an archive status with its entry count
type StatusFacet struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// CustomFieldFacet represents a custom field with its value distribution
type CustomFieldFacet struct {
	FieldName string        `json:"field_name"`
	Values    []ValueCount  `json:"values"`
}

// ValueCount represents a field value with its occurrence count
type ValueCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// DateRangeFacet represents the min/max date range of entries
type DateRangeFacet struct {
	Min string `json:"min"`
	Max string `json:"max"`
}