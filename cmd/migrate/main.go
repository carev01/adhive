package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var (
	dir    = flag.String("dir", "migrations", "migrations directory")
	dbPath = flag.String("db", "ad-catalog.db", "database path")
)

type Migration struct {
	ID   string
	Up   string
	Down string
}

func main() {
	action := flag.String("action", "up", "action: up, down, create, status")
	name := flag.String("name", "", "migration name (for create)")
	flag.Parse()

	gormDB, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open GORM: %v", err)
	}

	// Get underlying SQL DB for direct access
	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("failed to get database: %v", err)
	}
	defer sqlDB.Close()

	switch *action {
	case "up":
		if err := runMigrations(gormDB, *dir); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
	case "down":
		if err := rollbackLastMigration(gormDB, *dir); err != nil {
			log.Fatalf("rollback failed: %v", err)
		}
	case "create":
		if *name == "" {
			log.Fatal("migration name required for create")
		}
		if err := createMigration(*name, *dir); err != nil {
			log.Fatalf("create failed: %v", err)
		}
	case "status":
		if err := showStatus(gormDB, *dir); err != nil {
			log.Fatalf("status failed: %v", err)
		}
	default:
		log.Fatalf("unknown action: %s", *action)
	}
}

func runMigrations(db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Create migrations tracking table
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get applied migrations
	var applied []string
	db.Model(&struct{ ID string }{}).Table("schema_migrations").Pluck("id", &applied)
	appliedSet := make(map[string]bool)
	for _, m := range applied {
		appliedSet[m] = true
	}

	// Apply pending migrations
	for _, m := range migrations {
		if appliedSet[m.ID] {
			fmt.Printf("Skipping %s (already applied)\n", m.ID)
			continue
		}

		fmt.Printf("Applying %s...\n", m.ID)
		if err := db.Exec(m.Up).Error; err != nil {
			return fmt.Errorf("apply %s: %w", m.ID, err)
		}

		if err := db.Exec("INSERT INTO schema_migrations (id) VALUES (?)", m.ID).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", m.ID, err)
		}
		fmt.Printf("✓ %s applied\n", m.ID)
	}

	fmt.Println("Migrations complete")
	return nil
}

func rollbackLastMigration(db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Get last applied migration
	var lastID string
	row := db.Raw("SELECT id FROM schema_migrations ORDER BY applied_at DESC LIMIT 1").Row()
	if err := row.Scan(&lastID); err != nil {
		return fmt.Errorf("no migrations to rollback: %w", err)
	}

	// Find migration
	var migration Migration
	for _, m := range migrations {
		if m.ID == lastID {
			migration = m
			break
		}
	}

	if migration.ID == "" {
		return fmt.Errorf("migration %s not found", lastID)
	}

	fmt.Printf("Rolling back %s...\n", lastID)
	if err := db.Exec(migration.Down).Error; err != nil {
		return fmt.Errorf("rollback %s: %w", lastID, err)
	}

	if err := db.Exec("DELETE FROM schema_migrations WHERE id = ?", lastID).Error; err != nil {
		return fmt.Errorf("remove migration record: %w", err)
	}

	fmt.Printf("✓ %s rolled back\n", lastID)
	return nil
}

func loadMigrations(dir string) ([]Migration, error) {
	var migrations []Migration

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sql" {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			m := parseMigration(e.Name(), string(data))
			migrations = append(migrations, m)
		}
	}

	// Sort by ID
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].ID < migrations[j].ID
	})

	return migrations, nil
}

func parseMigration(filename, content string) Migration {
	id := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Simple split by -- +migrate marker
	parts := strings.Split(content, "-- +migrate ")

	migration := Migration{ID: id}

	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if strings.HasPrefix(part, "Up") {
			// Extract Up SQL (between Up and next marker or end)
			upContent := strings.TrimSpace(strings.TrimPrefix(part, "Up"))
			if idx := strings.Index(upContent, "-- +migrate"); idx > 0 {
				upContent = strings.TrimSpace(upContent[:idx])
			}
			migration.Up = upContent
		} else if strings.HasPrefix(part, "Down") {
			// Extract Down SQL
			downContent := strings.TrimSpace(strings.TrimPrefix(part, "Down"))
			if idx := strings.Index(downContent, "-- +migrate"); idx > 0 {
				downContent = strings.TrimSpace(downContent[:idx])
			}
			migration.Down = downContent
		}
	}

	return migration
}

func createMigration(name, dir string) error {
	// Find next sequence number
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	maxNum := 0
	for _, f := range files {
		var n int
		if _, err := fmt.Sscanf(f.Name(), "%d_", &n); err == nil {
			if n > maxNum {
				maxNum = n
			}
		}
	}

	filename := fmt.Sprintf("%0d_%s.sql", maxNum+1, name)
	content := `-- +migrate Up
-- Add your migration here

-- +migrate Down
-- Rollback here
`

	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Created migration: %s\n", filename)
	return nil
}

func showStatus(db *gorm.DB, dir string) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return err
	}

	// Ensure migrations table exists
	db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)

	var applied []string
	db.Raw("SELECT id FROM schema_migrations").Pluck("id", &applied)
	appliedSet := make(map[string]bool)
	for _, m := range applied {
		appliedSet[m] = true
	}

	fmt.Println("Migration Status:")
	fmt.Println("==================")
	for _, m := range migrations {
		status := "pending"
		if appliedSet[m.ID] {
			status = "applied"
		}
		fmt.Printf("  %s [%s]\n", m.ID, status)
	}
	return nil
}
