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
}

// DefaultStorageConfig returns default storage configuration
func DefaultStorageConfig() *StorageConfig {
	baseDir := os.Getenv("STORAGE_DIR")
	if baseDir == "" {
		baseDir = "./data"
	}
	
	return &StorageConfig{
		BaseDir:     baseDir,
		ArchivesDir: filepath.Join(baseDir, "archives"),
		ThumbDir:    filepath.Join(baseDir, "thumbnails"),
	}
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
