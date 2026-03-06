package test

import (
	"os"

	"github.com/carev01/adhive/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// NewInMemoryDB creates an in-memory SQLite database for testing
func NewInMemoryDB() (*gorm.DB, error) {
	// Use file-based SQLite for better compatibility
	// It will be automatically cleaned up
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// NewTempFileDB creates a temporary file-based SQLite database
func NewTempFileDB() (*gorm.DB, func(), error) {
	// Create temp file
	f, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		return nil, nil, err
	}
	dbPath := f.Name()
	f.Close()

	// Open database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		os.Remove(dbPath)
		return nil, nil, err
	}

	// Cleanup function
	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.Remove(dbPath)
	}

	return db, cleanup, nil
}

// AutoMigrate runs auto migration on the given database
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&model.User{}, &model.Session{})
}
