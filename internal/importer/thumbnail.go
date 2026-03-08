package importer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/service"
	"github.com/google/uuid"
)

// ThumbnailConverter handles conversion of Shiori thumbnails to AdHive format
type ThumbnailConverter struct {
	shioriThumbsDir string
	adhiveThumbsDir string
	dataDir         string
	thumbService    *service.ThumbnailService
}

// NewThumbnailConverter creates a new ThumbnailConverter
func NewThumbnailConverter(shioriThumbsDir, adhiveThumbsDir string) *ThumbnailConverter {
	// Calculate dataDir from adhiveThumbsDir (thumbnails is at dataDir/thumbnails)
	dataDir := filepath.Dir(adhiveThumbsDir)
	if filepath.Base(adhiveThumbsDir) == "thumbnails" {
		dataDir = filepath.Dir(adhiveThumbsDir)
	}
	
	return &ThumbnailConverter{
		shioriThumbsDir: shioriThumbsDir,
		adhiveThumbsDir: adhiveThumbsDir,
		dataDir:         dataDir,
		thumbService:    service.NewThumbnailService(dataDir),
	}
}

// ConvertThumbnail converts a single Shiori thumbnail to AdHive format
func (tc *ThumbnailConverter) ConvertThumbnail(shioriID int, adhiveEntryID string) (*model.ThumbnailCandidate, error) {
	// Find the source thumbnail file
	shioriThumbPath := filepath.Join(tc.shioriThumbsDir, strconv.Itoa(shioriID))

	// Try common extensions (including NO extension - Shiori uses no extension)
	extensions := []string{".jpg", ".jpeg", ".png", ""}
	var srcPath string

	for _, ext := range extensions {
		testPath := shioriThumbPath + ext
		if _, err := os.Stat(testPath); err == nil {
			srcPath = testPath
			break
		}
	}

	if srcPath == "" {
		// Return nil with no error - thumbnail doesn't exist for this ID
		// This is normal - not all entries have thumbnails
		return nil, nil
	}

	// Use ThumbnailService to convert to WebP with proper sizing
	result, err := tc.thumbService.SaveFromFile(srcPath, adhiveEntryID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert thumbnail to WebP: %w", err)
	}

	log.Printf("[DEBUG] Converted thumbnail: %s -> %s", srcPath, result.Path)

	// Create thumbnail candidate record
	// Path should be relative to dataDir for the handler to resolve correctly
	relPath := result.Path
	if filepath.IsAbs(relPath) {
		// Convert absolute path to relative from dataDir
		relPath, _ = filepath.Rel(tc.dataDir, relPath)
	}

	candidate := &model.ThumbnailCandidate{
		ID:         uuid.New().String(),
		EntryID:    adhiveEntryID,
		SourceType: model.ThumbnailCandidateSourceUpload,
		Path:       relPath,
		Score:      0.8, // High score for imported thumbnails
		Selected:   true, // Auto-select imported thumbnails
		CreatedAt:  time.Now(),
	}

	return candidate, nil
}

// ConvertAll converts all Shiori thumbnails for the given ID mappings
func (tc *ThumbnailConverter) ConvertAll(idMapping map[int]string) (int, int, []error) {
	successCount := 0
	skipCount := 0
	var errors []error

	for shioriID, adhiveID := range idMapping {
		candidate, err := tc.ConvertThumbnail(shioriID, adhiveID)
		if err != nil {
			log.Printf("[WARN] Failed to convert thumbnail for Shiori ID %d: %v", shioriID, err)
			errors = append(errors, err)
			skipCount++
			continue
		}
		
		// Skip silently if no thumbnail exists for this ID
		if candidate == nil {
			skipCount++
			continue
		}

		successCount++
	}

	return successCount, skipCount, errors
}
