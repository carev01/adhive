package migrations

import (
	"log"

	"gorm.io/gorm"
)

// V2CreateFTS5 creates FTS5 virtual table for full-text search
func V2CreateFTS5(db *gorm.DB) error {
	// Create FTS5 virtual table
	ftsSQL := `
		CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
			title,
			description,
			phone_number,
			location,
			url,
			content=catalog_entries,
			content_rowid=rowid
		);
	`

	if err := db.Exec(ftsSQL).Error; err != nil {
		log.Printf("Warning: Failed to create FTS5 table: %v", err)
		return err
	}
	log.Printf("FTS5 virtual table created: entries_fts")

	// Create triggers to keep FTS in sync
	triggers := []string{
		// AFTER INSERT trigger
		`CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON catalog_entries BEGIN
			INSERT INTO entries_fts(rowid, title, description, phone_number, location, url)
			VALUES (NEW.rowid, NEW.title, NEW.description, NEW.phone_number, NEW.location, NEW.url);
		END;`,

		// AFTER DELETE trigger
		`CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON catalog_entries BEGIN
			INSERT INTO entries_fts(entries_fts, rowid, title, description, phone_number, location, url)
			VALUES ('delete', OLD.rowid, OLD.title, OLD.description, OLD.phone_number, OLD.location, OLD.url);
		END;`,

		// AFTER UPDATE trigger
		`CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE ON catalog_entries BEGIN
			INSERT INTO entries_fts(entries_fts, rowid, title, description, phone_number, location, url)
			VALUES ('delete', OLD.rowid, OLD.title, OLD.description, OLD.phone_number, OLD.location, OLD.url);
			INSERT INTO entries_fts(rowid, title, description, phone_number, location, url)
			VALUES (NEW.rowid, NEW.title, NEW.description, NEW.phone_number, NEW.location, NEW.url);
		END;`,
	}

	for _, trigger := range triggers {
		if err := db.Exec(trigger).Error; err != nil {
			log.Printf("Warning: Failed to create FTS trigger: %v", err)
			// Don't fail - continue
		} else {
			log.Printf("FTS trigger created")
		}
	}

	return nil
}

// RepopulateFTS5 populates FTS5 table from existing data
func RepopulateFTS5(db *gorm.DB) error {
	// Clear existing FTS data
	if err := db.Exec("DELETE FROM entries_fts").Error; err != nil {
		log.Printf("Warning: Failed to clear FTS table: %v", err)
	}

	// Repopulate from catalog_entries
	repopulateSQL := `
		INSERT INTO entries_fts(rowid, title, description, phone_number, location, url)
		SELECT rowid, title, description, phone_number, location, url FROM catalog_entries;
	`

	if err := db.Exec(repopulateSQL).Error; err != nil {
		log.Printf("Warning: Failed to repopulate FTS table: %v", err)
		return err
	}

	log.Printf("FTS table repopulated with existing data")
	return nil
}
