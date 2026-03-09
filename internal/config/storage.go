package config

import (
	"os"
	"path/filepath"
	"strings"
)

// StorageConfig holds file storage configuration
type StorageConfig struct {
	BaseDir     string
	ArchivesDir string
	ThumbDir    string
	DataDir     string // New unified data directory
	DBPath      string // Database path
}

// DefaultStorageConfig returns default storage configuration
// Supports unified DATA_DIR or individual DB_PATH/STORAGE_DIR for backwards compatibility
func DefaultStorageConfig() *StorageConfig {
	// Check for unified DATA_DIR first
	dataDir := os.Getenv("DATA_DIR")
	
	var baseDir, dbPath string
	
	if dataDir != "" {
		// Use unified DATA_DIR
		baseDir = dataDir
		dbPath = filepath.Join(dataDir, "adhive.db")
	} else {
		// Fall back to individual paths for backwards compatibility
		baseDir = os.Getenv("STORAGE_DIR")
		if baseDir == "" {
			baseDir = "./data"
		}
		
		dbPath = os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "ad-catalog.db"
		}
	}
	
	return &StorageConfig{
		BaseDir:     baseDir,
		ArchivesDir: filepath.Join(baseDir, "archives"),
		ThumbDir:    filepath.Join(baseDir, "thumbnails"),
		DataDir:     dataDir,
		DBPath:      dbPath,
	}
}

// GetDBPath returns the database path
// Supports DATA_DIR (unified) or DB_PATH (backwards compatible)
func GetDBPath() string {
	// Check for unified DATA_DIR first
	dataDir := os.Getenv("DATA_DIR")
	if dataDir != "" {
		return filepath.Join(dataDir, "adhive.db")
	}
	
	// Fall back to DB_PATH
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "ad-catalog.db"
	}
	return dbPath
}

// EnsureDirectories creates storage directories if they don't exist
func (s *StorageConfig) EnsureDirectories() error {
	dirs := []string{
		s.BaseDir,
		s.ArchivesDir,
		s.ThumbDir,
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// ArchivePath returns the full path for an entry's archive directory
func (s *StorageConfig) ArchivePath(entryID string) string {
	return filepath.Join(s.ArchivesDir, entryID)
}

// ThumbnailPath returns the full path for an entry's thumbnail
func (s *StorageConfig) ThumbnailPath(entryID string) string {
	// Look for any .webp file in the thumbnails directory for this entry
	thumbDir := filepath.Join(s.ThumbDir, entryID)
	if entries, err := os.ReadDir(thumbDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".webp") {
				return filepath.Join(thumbDir, e.Name())
			}
		}
	}
	// Fallback to old format (entryID.webp)
	return filepath.Join(s.ThumbDir, entryID+".webp")
}
