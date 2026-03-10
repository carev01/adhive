package importtool

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

// BoltExtractor extracts content from Shiori's BoltDB archive files
type BoltExtractor struct {
	shioriArchiveDir string
	adhiveDataDir    string
}

// NewBoltExtractor creates a new BoltDB extractor
func NewBoltExtractor(shioriArchiveDir, adhiveDataDir string) *BoltExtractor {
	return &BoltExtractor{
		shioriArchiveDir: shioriArchiveDir,
		adhiveDataDir:    adhiveDataDir,
	}
}

// ExtractResult contains the result of extracting a single archive
type ExtractResult struct {
	BookmarkID  string          `json:"bookmark_id"`
	EntryID     string          `json:"entry_id"`
	RevisionID  string          `json:"revision_id"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	HTMLSize    int64           `json:"html_size"`
	AssetsCount int             `json:"assets_count"`
	Assets      []ManifestAsset `json:"assets"`
}

// Manifest represents the archive manifest
type Manifest struct {
	Version     string          `json:"version"`
	OriginalURL string          `json:"original_url"`
	CapturedAt  time.Time       `json:"captured_at"`
	Engine      string          `json:"engine"`
	Assets      []ManifestAsset `json:"assets"`
}

// ManifestAsset represents a single asset in the archive
type ManifestAsset struct {
	SourceURL   string `json:"source_url"`
	LocalPath   string `json:"local_path"`
	ContentHash string `json:"content_hash,omitempty"`
	MimeType    string `json:"mime_type"`
	Bytes       int64  `json:"bytes"`
	Kind        string `json:"kind"`
}

// ExtractAll extracts all archives from Shiori to AdHive format
func (e *BoltExtractor) ExtractAll(ctx context.Context, bookmarkIDToEntryID map[string]string) ([]ExtractResult, error) {
	var results []ExtractResult

	entries, err := os.ReadDir(e.shioriArchiveDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read Shiori archive dir: %w", err)
	}

	for _, entry := range entries {
		// Shiori stores archives as files (BoltDB), not directories
		// Each file named by bookmark ID contains the archived content
		if entry.IsDir() {
			continue // Skip directories
		}

		bookmarkID := entry.Name()
		entryID, ok := bookmarkIDToEntryID[bookmarkID]
		if !ok {
			// If no mapping provided, use bookmarkID as entryID
			entryID = bookmarkID
		}

		result := e.ExtractArchive(ctx, bookmarkID, entryID)
		results = append(results, result)

		if len(results)%10 == 0 {
			log.Printf("Progress: %d/%d archives processed", len(results), len(entries))
		}
	}

	return results, nil
}

// ExtractArchive extracts a single Shiori archive to AdHive format
// Creates archives indistinguishable from native AdHive archives
func (e *BoltExtractor) ExtractArchive(ctx context.Context, bookmarkID, entryID string) ExtractResult {
	result := ExtractResult{
		BookmarkID: bookmarkID,
		EntryID:    entryID,
	}

	archivePath := filepath.Join(e.shioriArchiveDir, bookmarkID)

	// Find the BoltDB file (usually the bookmark ID)
	dbPath := archivePath
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Try with .db extension
		dbPath = archivePath + ".db"
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			result.Error = "archive file not found"
			return result
		}
	}

	// Open BoltDB directly
	db, err := bbolt.Open(dbPath, 0444, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to open archive: %v", err)
		return result
	}

	// Extract main HTML content from archive-root bucket
	var htmlContent []byte
	var contentType string
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("archive-root"))
		if bucket == nil {
			return fmt.Errorf("archive-root bucket not found")
		}
		htmlContent = bucket.Get([]byte("content"))
		contentType = string(bucket.Get([]byte("type")))
		htmlContent = decompressGzip(htmlContent, contentType)
		return nil
	})
	if err != nil {
		db.Close()
		result.Error = fmt.Sprintf("failed to read archive-root: %v", err)
		return result
	}
	db.Close()

	if htmlContent == nil {
		result.Error = "no content in archive-root"
		return result
	}

	result.HTMLSize = int64(len(htmlContent))

	// Create AdHive archive directory structure
	// Format: data/archives/{entryID}/rev-0001/
	revisionID := generateUUID()
	revisionDir := filepath.Join(e.adhiveDataDir, "archives", entryID, "rev-0001")
	assetsDir := filepath.Join(revisionDir, "assets")
	metaDir := filepath.Join(revisionDir, "meta")

	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		result.Error = fmt.Sprintf("failed to create revision dirs: %v", err)
		return result
	}
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		result.Error = fmt.Sprintf("failed to create meta dir: %v", err)
		return result
	}

	// Extract assets first (need mapping for URL rewriting)
	assetMapping, err := e.extractAssets(dbPath, assetsDir)
	if err != nil {
		log.Printf("Warning: failed to extract some assets: %v", err)
	}
	result.AssetsCount = len(assetMapping)
	result.Assets = assetMapping

	// Rewrite URLs in HTML to point to local assets
	rewrittenHTML := rewriteHTMLURLs(htmlContent, assetMapping)

	// Write index.html
	indexPath := filepath.Join(revisionDir, "index.html")
	if err := os.WriteFile(indexPath, rewrittenHTML, 0644); err != nil {
		result.Error = fmt.Sprintf("failed to write index.html: %v", err)
		return result
	}

	// Create meta/info.json
	metaPath := filepath.Join(metaDir, "info.json")
	metaJSON := fmt.Sprintf(`{
  "revision_id": "%s",
  "entry_id": "%s",
  "engine": "shiori-migrated",
  "captured_at": "%s"
}`, revisionID, entryID, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(metaPath, []byte(metaJSON), 0644); err != nil {
		log.Printf("Warning: failed to write meta info: %v", err)
	}

	// Create manifest
	manifest := Manifest{
		Version:     "1.0",
		OriginalURL: "", // Would need to look up from bookmarks
		CapturedAt:  time.Now(),
		Engine:      "shiori-migrated",
		Assets:      assetMapping,
	}

	manifestPath := filepath.Join(revisionDir, "manifest.json")
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
		log.Printf("Warning: failed to write manifest: %v", err)
	}

	result.RevisionID = revisionID
	result.Success = true

	return result
}

// extractAssets extracts all assets from the archive to assetsDir
func (e *BoltExtractor) extractAssets(archivePath, assetsDir string) ([]ManifestAsset, error) {
	var assets []ManifestAsset

	// Open BoltDB directly (warc library doesn't expose internal db)
	db, err := bbolt.Open(archivePath, 0444, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}
	defer db.Close()

	err = db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			bucketName := string(name)

			// Skip the archive-root bucket (already handled)
			if bucketName == "archive-root" || bucketName == "archive" {
				return nil
			}

			// Get content
			content := bucket.Get([]byte("content"))
			if len(content) == 0 {
				return nil
			}

			// Get content type
			contentType := string(bucket.Get([]byte("type")))

			// Decompress if needed
			content = decompressGzip(content, contentType)

			// Determine file extension and kind
			ext := getExtension(contentType)
			kind := getKind(contentType)

			// Generate filename from bucket name (hash-based for uniqueness)
			safeName := sanitizeFilename(bucketName)
			assetPath := filepath.Join(assetsDir, safeName+ext)

			// Write asset file
			err := os.WriteFile(assetPath, content, 0644)
			if err != nil {
				log.Printf("Warning: failed to write asset %s: %v", bucketName, err)
				return nil
			}

			// Calculate hash
			hash := sha256.Sum256(content)
			contentHash := hex.EncodeToString(hash[:])

			assets = append(assets, ManifestAsset{
				SourceURL:   bucketName,
				LocalPath:   "assets/" + safeName + ext,
				ContentHash: contentHash,
				MimeType:    contentType,
				Bytes:       int64(len(content)),
				Kind:        kind,
			})

			return nil
		})
	})

	return assets, err
}

// decompressGzip decompresses gzip content if needed
func decompressGzip(content []byte, _ string) []byte {
	// Check for gzip magic bytes (1f 8b)
	if len(content) >= 2 && content[0] == 0x1f && content[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(content))
		if err == nil {
			defer reader.Close()
			decompressed, err := io.ReadAll(reader)
			if err == nil {
				return decompressed
			}
		}
	}
	return content
}

// rewriteHTMLURLs replaces original URLs in HTML with local asset paths
func rewriteHTMLURLs(html []byte, assets []ManifestAsset) []byte {
	if len(assets) == 0 {
		return html
	}

	result := string(html)

	// Build a map of source URL to local path
	urlMap := make(map[string]string)
	for _, asset := range assets {
		urlMap[asset.SourceURL] = asset.LocalPath
	}

	// Replace each URL in the HTML
	for sourceURL, localPath := range urlMap {
		// Replace in src and href attributes
		// Note: localPath already includes 'assets/' prefix from extractAssets, so don't add it again
		result = strings.ReplaceAll(result, fmt.Sprintf("src=\"%s\"", sourceURL), fmt.Sprintf("src=\"%s\"", localPath))
		result = strings.ReplaceAll(result, fmt.Sprintf("href=\"%s\"", sourceURL), fmt.Sprintf("href=\"%s\"", localPath))

		// Handle URLs without quotes (less common but possible)
		result = strings.ReplaceAll(result, fmt.Sprintf("src='%s'", sourceURL), fmt.Sprintf("src='%s'", localPath))
		result = strings.ReplaceAll(result, fmt.Sprintf("href='%s'", sourceURL), fmt.Sprintf("href='%s'", localPath))
	}

	return []byte(result)
}

// getExtension returns the file extension based on content type
func getExtension(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/html"):
		return ".html"
	case strings.Contains(ct, "text/css"):
		return ".css"
	case strings.Contains(ct, "image/jpeg"), strings.Contains(ct, "image/jpg"):
		return ".jpg"
	case strings.Contains(ct, "image/png"):
		return ".png"
	case strings.Contains(ct, "image/gif"):
		return ".gif"
	case strings.Contains(ct, "image/svg"):
		return ".svg"
	case strings.Contains(ct, "application/javascript"), strings.Contains(ct, "text/javascript"):
		return ".js"
	case strings.Contains(ct, "application/json"):
		return ".json"
	case strings.Contains(ct, "font/woff"):
		return ".woff"
	case strings.Contains(ct, "font/woff2"):
		return ".woff2"
	case strings.Contains(ct, "image/webp"):
		return ".webp"
	default:
		return ".bin"
	}
}

// getKind returns the asset kind based on content type
func getKind(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/css"):
		return "css"
	case strings.Contains(ct, "application/javascript"), strings.Contains(ct, "text/javascript"):
		return "js"
	case strings.Contains(ct, "image/"):
		return "img"
	case strings.Contains(ct, "font/"):
		return "font"
	case strings.Contains(ct, "video/"), strings.Contains(ct, "audio/"):
		return "media"
	default:
		return "other"
	}
}

// sanitizeFilename creates a safe filename from a bucket name
func sanitizeFilename(name string) string {
	// Replace common URL separators with underscores
	safe := strings.ReplaceAll(name, "/", "_")
	safe = strings.ReplaceAll(safe, "-", "_")
	safe = strings.ReplaceAll(safe, ".", "_")

	// Remove any remaining unsafe characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	safe = reg.ReplaceAllString(safe, "")

	// Limit length
	if len(safe) > 100 {
		safe = safe[:100]
	}

	return safe
}

// generateUUID generates a simple UUID-like string
// In production, use github.com/google/uuid
func generateUUID() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:])[:32]
}
