package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/carev01/adhive/internal/config"
)

// FileService handles file storage operations
type FileService struct {
	storage *config.StorageConfig
}

// NewFileService creates a new FileService
func NewFileService(storage *config.StorageConfig) *FileService {
	return &FileService{storage: storage}
}

// GetConfig returns the storage configuration
func (s *FileService) GetConfig() *config.StorageConfig {
	return s.storage
}

// InitStorage initializes storage directories
func (s *FileService) InitStorage() error {
	return s.storage.EnsureDirectories()
}

// SaveArchive saves an uploaded archive file for an entry
func (s *FileService) SaveArchive(entryID string, file *multipart.FileHeader) (string, error) {
	archiveDir := s.storage.ArchivePath(entryID)
	
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create archive directory: %w", err)
	}
	
	filename := filepath.Base(file.Filename)
	dst := filepath.Join(archiveDir, filename)
	
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()
	
	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()
	
	if _, err := io.Copy(out, src); err != nil {
		return "", fmt.Errorf("failed to save archive: %w", err)
	}
	
	return dst, nil
}

// SaveThumbnail saves an uploaded thumbnail for an entry
func (s *FileService) SaveThumbnail(entryID string, file *multipart.FileHeader) (string, error) {
	// Create entry-specific subdirectory for consistency with SelectCandidate
	thumbDir := filepath.Join(s.storage.ThumbDir, entryID)
	
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create thumbnail directory: %w", err)
	}
	
	dst := filepath.Join(thumbDir, entryID+".webp")
	
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()
	
	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()
	
	if _, err := io.Copy(out, src); err != nil {
		return "", fmt.Errorf("failed to save thumbnail: %w", err)
	}
	
	return dst, nil
}

// GetArchive retrieves an archive file
func (s *FileService) GetArchive(entryID, filename string) ([]byte, string, error) {
	path := filepath.Join(s.storage.ArchivePath(entryID), filename)
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	
	contentType := http.DetectContentType(data)
	return data, contentType, nil
}

// GetArchiveFromPath retrieves an archive file from an arbitrary path
func (s *FileService) GetArchiveFromPath(archivePath, filename string) ([]byte, string, error) {
	// Handle relative paths (e.g., "data/archives/userID/filename.html")
	dir := archivePath
	if !filepath.IsAbs(dir) {
		// Get directory from file path
		dir = filepath.Dir(archivePath)
		// Resolve relative to current working directory
		cwd, _ := os.Getwd()
		dir = filepath.Join(cwd, dir)
	}
	
	path := filepath.Join(dir, filename)
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	
	contentType := http.DetectContentType(data)
	return data, contentType, nil
}

// GetThumbnail retrieves a thumbnail file
func (s *FileService) GetThumbnail(entryID string) ([]byte, string, error) {
	path := s.storage.ThumbnailPath(entryID)
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	
	contentType := http.DetectContentType(data)
	return data, contentType, nil
}

// DeleteArchive removes archive files for an entry
func (s *FileService) DeleteArchive(entryID string) error {
	dir := s.storage.ArchivePath(entryID)
	return os.RemoveAll(dir)
}

// DeleteThumbnail removes thumbnail for an entry
func (s *FileService) DeleteThumbnail(entryID string) error {
	path := s.storage.ThumbnailPath(entryID)
	return os.Remove(path)
}

// ListArchives lists all files in an entry's archive directory
func (s *FileService) ListArchives(entryID string) ([]string, error) {
	dir := s.storage.ArchivePath(entryID)
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	
	return files, nil
}

// ListArchivesFromPath lists files in an arbitrary archive path
func (s *FileService) ListArchivesFromPath(archivePath string) ([]string, error) {
	// Handle relative paths (e.g., "data/archives/userID/filename.html")
	dir := archivePath
	if !filepath.IsAbs(dir) {
		// Get directory from file path
		dir = filepath.Dir(archivePath)
		// Resolve relative to current working directory
		cwd, _ := os.Getwd()
		dir = filepath.Join(cwd, dir)
	}
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	
	return files, nil
}
