package migrations

import (
	"log"

	"gorm.io/gorm"
)

// V1CreateIndexes creates composite indexes for performance
func V1CreateIndexes(db *gorm.DB) error {
	indexes := []string{
		// User + created_at (ordered pagination)
		`CREATE INDEX IF NOT EXISTS idx_entries_user_created ON catalog_entries(user_id, created_at DESC);`,

		// User + status + created_at (status filtering + pagination)
		`CREATE INDEX IF NOT EXISTS idx_entries_user_status_created ON catalog_entries(user_id, archive_status, created_at DESC);`,

		// User + location (location filtering)
		`CREATE INDEX IF NOT EXISTS idx_entries_user_location ON catalog_entries(user_id, location);`,

		// Interactions (exclude_tried filter)
		`CREATE INDEX IF NOT EXISTS idx_interactions_user_tried ON interactions(user_id, tried, entry_id);`,

		// Interactions (score filter)
		`CREATE INDEX IF NOT EXISTS idx_interactions_user_score ON interactions(user_id, score);`,

		// Tags (ordered list)
		`CREATE INDEX IF NOT EXISTS idx_tags_user_name ON tags(user_id, name);`,

		// Entry-tag junction (both directions)
		`CREATE INDEX IF NOT EXISTS idx_entry_tags_entry_tag ON entry_tags(entry_id, tag_id);`,
		`CREATE INDEX IF NOT EXISTS idx_entry_tags_tag_entry ON entry_tags(tag_id, entry_id);`,
	}

	for _, idx := range indexes {
		if err := db.Exec(idx).Error; err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
			// Don't fail - indexes may already exist
		} else {
			log.Printf("Index created: %s", idx[:50]+"...")
		}
	}

	return nil
}
