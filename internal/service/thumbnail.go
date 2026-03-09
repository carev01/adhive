package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/carev01/adhive/internal/model"
	"github.com/google/uuid"
)

// ThumbnailService handles thumbnail extraction and storage
type ThumbnailService struct {
	dataDir    string
	maxWidth   int
	maxHeight  int
	httpClient *http.Client
}

// NewThumbnailService creates a new ThumbnailService
func NewThumbnailService(dataDir string) *ThumbnailService {
	return &ThumbnailService{
		dataDir:    dataDir,
		maxWidth:   300,
		maxHeight:  300,
		httpClient: &http.Client{Timeout: 30 * 10},
	}
}

// ThumbnailResult represents the result of thumbnail generation
type ThumbnailResult struct {
	Path      string
	Width     int
	Height    int
	WasCustom bool
}

// CandidateInput describes a possible thumbnail image source.
type CandidateInput struct {
	SourcePath string
	SourceURL  string
	SourceType model.ThumbnailCandidateSourceType
	RevisionID *string
}

// CandidateResult is the persisted/processed thumbnail candidate.
type CandidateResult struct {
	Path       string
	Score      float64
	Width      int
	Height     int
	SourceURL  string
	SourceType model.ThumbnailCandidateSourceType
	RevisionID *string
}

// ExtractFromURL extracts a thumbnail from a URL's page
func (s *ThumbnailService) ExtractFromURL(pageURL, entryID string) (*ThumbnailResult, error) {
	// Fetch the page
	resp, err := s.httpClient.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	// Read HTML to find image
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	// Extract image URL from HTML
	imageURL := s.extractImageURL(string(html), pageURL)
	if imageURL == "" {
		return nil, fmt.Errorf("no image found on page")
	}

	// Download image
	imageData, err := s.downloadImage(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	// Process and save
	return s.SaveFromData(imageData, entryID)
}

// SaveFromData saves thumbnail from image data
func (s *ThumbnailService) SaveFromData(imageData []byte, entryID string) (*ThumbnailResult, error) {
	return s.saveFromDataToDir(imageData, entryID, "", false)
}

func (s *ThumbnailService) saveFromDataToDir(imageData []byte, entryID, outputDir string, wasCustom bool) (*ThumbnailResult, error) {
	// Decode image
	img, format, err := decodeImageFirstFrame(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize to thumbnail
	thumbnail := imaging.Thumbnail(img, s.maxWidth, s.maxHeight, imaging.Lanczos)

	// Create output path
	thumbnailDir := outputDir
	if thumbnailDir == "" {
		thumbnailDir = filepath.Join(s.dataDir, "thumbnails", entryID)
	}
	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create thumbnail directory: %w", err)
	}

	filename := fmt.Sprintf("%s.webp", uuid.New().String()[:8])
	thumbnailPath := filepath.Join(thumbnailDir, filename)

	// Save as WebP (or fallback to PNG)
	if err := s.saveImage(thumbnail, thumbnailPath, format); err != nil {
		return nil, fmt.Errorf("failed to save thumbnail: %w", err)
	}

	bounds := thumbnail.Bounds()
	return &ThumbnailResult{
		Path:      thumbnailPath,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		WasCustom: wasCustom,
	}, nil
}

// SaveFromDataURL downloads an image from URL or decodes data: URL and creates thumbnail
func (s *ThumbnailService) SaveFromDataURL(imageURL, entryID string) (*ThumbnailResult, error) {
	var imageData []byte
	var err error

	// Check if it's a data: URL (base64 encoded)
	if strings.HasPrefix(imageURL, "data:") {
		imageData, err = s.decodeDataURL(imageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to decode data URL: %w", err)
		}
	} else {
		// Download image
		imageData, err = s.downloadImage(imageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download image: %w", err)
		}
	}

	// Process and save
	return s.SaveFromData(imageData, entryID)
}

// decodeDataURL decodes a data: URL to raw image bytes
func (s *ThumbnailService) decodeDataURL(dataURL string) ([]byte, error) {
	// Format: data:image/png;base64,<data>
	// or: data:image/jpeg;base64,<data>
	// or: data:image/gif;base64,<data>

	// Find the comma separator
	commaIdx := strings.Index(dataURL, ",")
	if commaIdx == -1 {
		return nil, fmt.Errorf("invalid data URL format")
	}

	// Get the metadata part (before comma)
	metadata := dataURL[:commaIdx]
	// Get the base64 data (after comma)
	base64Data := dataURL[commaIdx+1:]

	// Check if it's base64 encoded
	if !strings.Contains(metadata, "base64") {
		// URL-encoded data - decode it
		decoded, err := url.QueryUnescape(base64Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode URL-encoded data: %w", err)
		}
		return []byte(decoded), nil
	}

	// Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	return decoded, nil
}

// SaveCustom saves a custom uploaded thumbnail
func (s *ThumbnailService) SaveCustom(reader io.Reader, userID string) (*ThumbnailResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	return s.saveFromDataToDir(data, userID, filepath.Join(s.dataDir, "thumbnails", userID), true)
}

// SaveFromFile saves thumbnail from an existing image file path.
func (s *ThumbnailService) SaveFromFile(sourcePath, entryID string) (*ThumbnailResult, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read source image: %w", err)
	}
	return s.saveFromDataToDir(data, entryID, "", false)
}

// BuildCandidatesFromLocalAssets scans archived local assets and creates scored thumbnail candidates.
func (s *ThumbnailService) BuildCandidatesFromLocalAssets(entryID string, inputs []CandidateInput) ([]CandidateResult, error) {
	if len(inputs) == 0 {
		return []CandidateResult{}, nil
	}

	results := make([]CandidateResult, 0, len(inputs))
	candidatesDir := filepath.Join(s.dataDir, "thumbnails", entryID, "candidates")
	if err := os.MkdirAll(candidatesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create candidates dir: %w", err)
	}

	for _, in := range inputs {
		if in.SourcePath == "" {
			continue
		}

		data, err := os.ReadFile(in.SourcePath)
		if err != nil {
			continue
		}

		img, _, err := decodeImageFirstFrame(data)
		if err != nil {
			continue
		}

		bounds := img.Bounds()
		width := bounds.Dx()
		height := bounds.Dy()
		if width < 120 || height < 120 {
			continue
		}

		res, err := s.saveFromDataToDir(data, entryID, candidatesDir, false)
		if err != nil {
			continue
		}

		score := scoreCandidate(in.SourcePath, width, height)
		results = append(results, CandidateResult{
			Path:       res.Path,
			Score:      score,
			Width:      width,
			Height:     height,
			SourceURL:  in.SourceURL,
			SourceType: in.SourceType,
			RevisionID: in.RevisionID,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, nil
}

func scoreCandidate(sourcePath string, width, height int) float64 {
	area := float64(width * height)
	aspect := float64(width) / float64(height)
	aspectPenalty := math.Abs(aspect - 1.35) // prefer landscape-ish images
	if aspectPenalty > 1.2 {
		aspectPenalty = 1.2
	}

	name := strings.ToLower(filepath.Base(sourcePath))
	keywordBoost := 0.0
	if strings.Contains(name, "hero") || strings.Contains(name, "cover") || strings.Contains(name, "main") {
		keywordBoost += 0.25
	}
	if strings.Contains(name, "logo") || strings.Contains(name, "icon") || strings.Contains(name, "avatar") || strings.Contains(name, "sprite") {
		keywordBoost -= 0.35
	}

	return (math.Log(area+1) / 12.0) + (1.0 - aspectPenalty) + keywordBoost
}

func decodeImageFirstFrame(data []byte) (image.Image, string, error) {
	if g, err := gif.DecodeAll(bytes.NewReader(data)); err == nil && len(g.Image) > 0 {
		return g.Image[0], "gif", nil
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	return img, format, err
}

// extractImageURL extracts the best image URL from HTML
func (s *ThumbnailService) extractImageURL(html, pageURL string) string {
	// Try og:image first
	if ogImage := extractMetaContent(html, "og:image"); ogImage != "" {
		return s.resolveURL(ogImage, pageURL)
	}

	// Try twitter:image
	if twitterImage := extractMetaContent(html, "twitter:image"); twitterImage != "" {
		return s.resolveURL(twitterImage, pageURL)
	}

	// Try to find first large image in img tags
	if imgSrc := extractFirstImage(html); imgSrc != "" {
		return s.resolveURL(imgSrc, pageURL)
	}

	return ""
}

// extractMetaContent extracts content from meta tags (supports both name and property)
func extractMetaContent(html, metaName string) string {
	// Try property attribute first (for og:*, twitter:*)
	patterns := []string{
		fmt.Sprintf(`<meta[^>]+property=["']%s["'][^>]+content=["']([^"']+)["']`, metaName),
		fmt.Sprintf(`<meta[^>]+content=["']([^"']+)["'][^>]+property=["']%s["']`, metaName),
		// Try name attribute (for twitter:image, etc)
		fmt.Sprintf(`<meta[^>]+name=["']%s["'][^>]+content=["']([^"']+)["']`, metaName),
		fmt.Sprintf(`<meta[^>]+content=["']([^"']+)["'][^>]+name=["']%s["']`, metaName),
	}

	for _, pattern := range patterns {
		if match := regexMatch(pattern, html); match != "" {
			return match
		}
	}
	return ""
}

// extractFirstImage finds the first img src that's likely a product image
func extractFirstImage(html string) string {
	// Simple pattern to find img src
	patterns := []string{
		`<img[^>]+src=["']([^"']+\.(?:jpg|jpeg|png|webp))["']`,
	}

	for _, pattern := range patterns {
		if match := regexMatch(pattern, html); match != "" {
			// Skip data URLs and small icons
			if strings.HasPrefix(match, "data:") || strings.Contains(match, "icon") || strings.Contains(match, "logo") {
				continue
			}
			return match
		}
	}
	return ""
}

// resolveURL resolves a relative URL to absolute
func (s *ThumbnailService) resolveURL(src, base string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return src
	}

	relURL, err := url.Parse(src)
	if err != nil {
		return src
	}

	return baseURL.ResolveReference(relURL).String()
}

// downloadImage downloads an image from URL
func (s *ThumbnailService) downloadImage(imageURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}

	// Set reasonable headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "image/webp,image/jpeg,image/png,*/*")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("image returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// saveImage saves an image to disk
func (s *ThumbnailService) saveImage(img image.Image, path, originalFormat string) error {
	// Try to save as WebP first
	// Note: Standard library doesn't support WebP encoding, so we use PNG as fallback
	// For true WebP support, we'd need bimg library with libvips

	// Save as PNG (most compatible)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return err
	}

	log.Printf("Thumbnail saved to %s", path)
	return nil
}

// Helper for regex matching
func regexMatch(pattern, s string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
