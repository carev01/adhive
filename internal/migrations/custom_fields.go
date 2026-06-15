package migrations

import (
	"log"

	"gorm.io/gorm"
)

// V3CreateCustomFields creates the custom_fields table for EAV-based custom field storage
func V3CreateCustomFields(db *gorm.DB) error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS custom_fields (
			id TEXT PRIMARY KEY,
			entry_id TEXT NOT NULL REFERENCES catalog_entries(id) ON DELETE CASCADE,
			field_name VARCHAR(100) NOT NULL,
			field_value TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	if err := db.Exec(createTableSQL).Error; err != nil {
		log.Printf("Warning: Failed to create custom_fields table: %v", err)
		return err
	}
	log.Printf("custom_fields table created")

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_custom_fields_entry_field ON custom_fields(entry_id, field_name);`,
		`CREATE INDEX IF NOT EXISTS idx_custom_fields_name_value ON custom_fields(field_name, field_value);`,
		`CREATE INDEX IF NOT EXISTS idx_custom_fields_entry_id ON custom_fields(entry_id);`,
	}

	for _, idx := range indexes {
		if err := db.Exec(idx).Error; err != nil {
			log.Printf("Warning: Failed to create custom_fields index: %v", err)
		} else {
			log.Printf("custom_fields index created: %s", idx[:60]+"...")
		}
	}

	return nil
}