package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// FileHandler handles file-related HTTP requests
type FileHandler struct {
	fileService            *service.FileService
	entryRepo              *repository.EntryRepository
	thumbnailCandidateRepo *repository.ThumbnailCandidateRepository
	archiveRevisionRepo    *repository.ArchiveRevisionRepository
}

func setArchiveSecurityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cross-Origin-Opener-Policy", "same-origin")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; media-src 'self' data:; script-src 'self' 'unsafe-inline'; frame-ancestors 'self'; base-uri 'none'; connect-src 'none';")
}

func cleanArchivePathPart(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}

	decoded := v
	for i := 0; i < 3; i++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return "", false
		}
		if next == decoded {
			break
		}
		decoded = next
	}

	if hasTraversalToken(v) || hasTraversalToken(decoded) {
		return "", false
	}

	decoded = strings.ReplaceAll(decoded, "\\", "/")
	decoded = strings.TrimPrefix(decoded, "/")
	decoded = filepath.Clean(decoded)
	if decoded == "." || strings.HasPrefix(decoded, "../") || strings.HasPrefix(decoded, "..\\") || filepath.IsAbs(decoded) {
		return "", false
	}

	clean := strings.TrimPrefix(filepath.ToSlash(decoded), "/")
	if clean == "" || clean == "." {
		return "", false
	}
	if hasTraversalToken(clean) {
		return "", false
	}
	return clean, true
}

func decodeOnce(v string) (string, bool) {
	decoded, err := url.PathUnescape(v)
	if err != nil {
		return "", false
	}
	return decoded, true
}

func hasTraversalToken(v string) bool {
	if v == "" {
		return false
	}
	l := strings.ToLower(v)
	if strings.Contains(l, "%2e") || strings.Contains(l, "%2f") || strings.Contains(l, "%5c") {
		return true
	}
	n := strings.ReplaceAll(l, "\\", "/")
	if strings.Contains(n, "../") || strings.HasPrefix(n, "..") || strings.Contains(n, "/..") {
		return true
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func hasTraversalInRequestPath(c *gin.Context) bool {
	candidates := []string{c.Request.RequestURI, c.Request.URL.Path, c.Request.URL.RawPath}
	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		if hasTraversalToken(raw) {
			return true
		}
		decoded := raw
		for i := 0; i < 3; i++ {
			next, err := url.PathUnescape(decoded)
			if err != nil {
				return true
			}
			if hasTraversalToken(next) {
				return true
			}
			if next == decoded {
				break
			}
			decoded = next
		}
	}
	return false
}

func isValidArchiveID(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	_, err := uuid.Parse(v)
	return err == nil
}

func isValidRevisionID(v string) bool {
	if v == "" {
		return false
	}
	_, err := uuid.Parse(v)
	return err == nil
}

// rewriteArchiveAssetLinks rewrites relative asset URLs in archived HTML to absolute API paths.
func rewriteArchiveAssetLinks(html string, revID, entryID string) string {
	// Rewrite relative paths like "assets/xxx.jpg" or "/assets/xxx.jpg" 
	// to "/api/v1/files/archive/{entryID}/{revisionID}/assets/xxx.jpg"
	
	// Pattern 1: src="assets/..." or src="/assets/..." (no leading slash)
	re1 := regexp.MustCompile(`src=["'](assets/[^"']*)["']`)
	result := re1.ReplaceAllStringFunc(html, func(match string) string {
		return `src="/api/v1/files/archive/` + entryID + `/` + revID + `/` + match[5:] // remove src="
	})
	
	// Pattern 2: href="assets/..." or href="/assets/..."
	re2 := regexp.MustCompile(`href=["'](assets/[^"']*)["']`)
	result = re2.ReplaceAllStringFunc(result, func(match string) string {
		return `href="/api/v1/files/archive/` + entryID + `/` + revID + `/` + match[6:] // remove href="
	})

	return result
}

func (h *FileHandler) loadRevisionManifest(entryID, revisionID string) (*model.ArchiveManifest, error) {
	manifestPath := filepath.Join(h.fileService.GetConfig().ArchivesDir, entryID, revisionID, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest model.ArchiveManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// NewFileHandler creates a new FileHandler
func NewFileHandler(fileService *service.FileService, entryRepo *repository.EntryRepository, thumbnailCandidateRepo *repository.ThumbnailCandidateRepository) *FileHandler {
	return &FileHandler{
		fileService:            fileService,
		entryRepo:              entryRepo,
		thumbnailCandidateRepo: thumbnailCandidateRepo,
	}
}

// NewFileHandlerWithRevisionRepo creates a FileHandler with archive revision repository
func NewFileHandlerWithRevisionRepo(fileService *service.FileService, entryRepo *repository.EntryRepository, thumbnailCandidateRepo *repository.ThumbnailCandidateRepository, archiveRevisionRepo *repository.ArchiveRevisionRepository) *FileHandler {
	return &FileHandler{
		fileService:            fileService,
		entryRepo:              entryRepo,
		thumbnailCandidateRepo: thumbnailCandidateRepo,
		archiveRevisionRepo:    archiveRevisionRepo,
	}
}

// InitStorage initializes storage directories
func (h *FileHandler) InitStorage() error {
	return h.fileService.InitStorage()
}

// UploadArchive handles POST /api/v1/files/archives/:entryID
func (h *FileHandler) UploadArchive(c *gin.Context) {
	entryID := c.Param("entryID")
	if entryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entry_id required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}

	path, err := h.fileService.SaveArchive(entryID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": path})
}

// UploadThumbnail handles POST /api/v1/files/thumbnail
func (h *FileHandler) UploadThumbnail(c *gin.Context) {
	entryID := c.Param("entryID")
	if entryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entry_id required"})
		return
	}
	if hasTraversalToken(entryID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traversal payload"})
		return
	}
	if once, ok := decodeOnce(entryID); ok && hasTraversalToken(once) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traversal payload"})
		return
	}
	if !isValidArchiveID(entryID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry_id"})
		return
	}

	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}

	if _, err := h.fileService.SaveThumbnail(entryID, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save thumbnail"})
		return
	}

	// Update entry with thumbnail URL path (never filesystem path)
	entry.ThumbnailPath = "/api/v1/files/thumbnails/" + entryID
	entry.ThumbnailSource = model.ThumbnailSourceUpload
	if err := h.entryRepo.Update(c.Request.Context(), entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update entry"})
		return
	}

	thumbnailURL := "/api/v1/files/thumbnails/" + entryID
	c.JSON(http.StatusOK, gin.H{"path": thumbnailURL})
}

// GetArchive handles GET /api/v1/files/archive/:entryID (latest index)
// and GET /api/v1/files/archive/:entryID/:revisionID/*path (specific revision asset)
func (h *FileHandler) GetArchive(c *gin.Context) {
	if hasTraversalInRequestPath(c) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}

	entryID := c.Param("entryID")
	revisionID := c.Param("revisionID")
	rawPath := c.Param("path")

	if hasTraversalToken(c.Param("entryID")) || hasTraversalToken(c.Param("revisionID")) || hasTraversalToken(c.Param("path")) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}
	if once, ok := decodeOnce(entryID); ok && hasTraversalToken(once) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}
	if once, ok := decodeOnce(revisionID); ok && hasTraversalToken(once) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}
	if once, ok := decodeOnce(rawPath); ok && hasTraversalToken(once) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}

	if entryID == "" {
		c.Data(http.StatusBadRequest, "text/plain", []byte("entry_id required"))
		return
	}
	if !isValidArchiveID(entryID) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid entry_id"))
		return
	}

	setArchiveSecurityHeaders(c)

	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("entry not found"))
		return
	}

	// Check for ?rev= query param (force specific revision)
	if revParam := c.Query("rev"); revParam != "" && isValidRevisionID(revParam) {
		revisionID = revParam
		// Store in context for asset rewriting
		c.Set("forced_revision_id", revParam)
	}

	// latest mode: /api/v1/files/archive/:entryID
	if revisionID == "" {
		if entry.ArchiveCurrentRevisionID != nil && *entry.ArchiveCurrentRevisionID != "" {
			revisionID = *entry.ArchiveCurrentRevisionID
			// Store in context for asset rewriting
			c.Set("forced_revision_id", revisionID)
		} else {
			// legacy fallback
			files, listErr := h.fileService.ListArchives(entryID)
			if listErr == nil && len(files) > 0 {
				data, contentType, getErr := h.fileService.GetArchive(entryID, files[0])
				if getErr == nil {
					c.Data(http.StatusOK, contentType, data)
					return
				}
			}
			if entry.ArchivePath != "" {
				legacyFiles, _ := h.fileService.ListArchivesFromPath(entry.ArchivePath)
				if len(legacyFiles) > 0 {
					data, contentType, getErr := h.fileService.GetArchiveFromPath(entry.ArchivePath, legacyFiles[0])
					if getErr == nil {
						c.Data(http.StatusOK, contentType, data)
						return
					}
				}
			}
			c.Data(http.StatusNotFound, "text/plain", []byte("archive not found"))
			return
		}
		rawPath = "/index.html"
	}

	revisionID, ok := cleanArchivePathPart(revisionID)
	if !ok {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid revision id"))
		return
	}

	// Store revision ID in context for asset rewriting
	if revisionID != "" {
		c.Set("forced_revision_id", revisionID)
	}

	// Use database root_path if revision repo is available
	var revisionRoot string
	var manifestPath string
	var revision *model.ArchiveRevision
	if h.archiveRevisionRepo != nil {
		var err error
		revision, err = h.archiveRevisionRepo.GetByID(c.Request.Context(), revisionID)
		if err != nil || revision == nil {
			c.Data(http.StatusNotFound, "text/plain", []byte("revision not found"))
			return
		}
		revisionRoot = revision.RootPath
		manifestPath = revision.ManifestPath
	} else {
		revisionRoot = filepath.Join(h.fileService.GetConfig().ArchivesDir, entryID, revisionID)
		manifestPath = filepath.Join(revisionRoot, "manifest.json")
	}

	if strings.TrimSpace(rawPath) == "" {
		rawPath = "/index.html"
	}
	assetPath, ok := cleanArchivePathPart(rawPath)
	if !ok {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}

	var manifest *model.ArchiveManifest
	if manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				c.Data(http.StatusNotFound, "text/plain", []byte("revision not found"))
				return
			}
			c.Data(http.StatusInternalServerError, "text/plain", []byte("failed to load manifest"))
			return
		}
		var m model.ArchiveManifest
		if err := json.Unmarshal(data, &m); err != nil {
			c.Data(http.StatusInternalServerError, "text/plain", []byte("failed to parse manifest"))
			return
		}
		manifest = &m
	} else {
		c.Data(http.StatusInternalServerError, "text/plain", []byte("manifest path not available"))
		return
	}

	fullPath := filepath.Join(revisionRoot, filepath.FromSlash(assetPath))
	cleanRoot := filepath.Clean(revisionRoot) + string(filepath.Separator)
	cleanFull := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanFull, cleanRoot) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path traversal"))
		return
	}

	data, readErr := os.ReadFile(cleanFull)
	if readErr != nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("asset not found"))
		return
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(assetPath, ".html") {
		contentType = "text/html; charset=utf-8"
		// Rewrite asset links to include revision query param
		if forcedRev := c.GetString("forced_revision_id"); forcedRev != "" {
			data = []byte(rewriteArchiveAssetLinks(string(data), forcedRev, entryID))
		}
	} else if strings.HasSuffix(assetPath, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(assetPath, ".js") {
		contentType = "application/javascript"
	}

	// Surface archive quality hints.
	c.Header("X-Archive-Status", string(manifest.Status))
	c.Header("X-Archive-Revision", revisionID)
	c.Data(http.StatusOK, contentType, data)
}

// GetThumbnail handles GET /api/v1/files/thumbnail/:entryID
func (h *FileHandler) GetThumbnail(c *gin.Context) {
	entryID := c.Param("entryID")
	if hasTraversalToken(entryID) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}
	if once, ok := decodeOnce(entryID); ok && hasTraversalToken(once) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid traversal payload"))
		return
	}
	if !isValidArchiveID(entryID) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid entry_id"))
		return
	}

	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err != nil || entry == nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("entry not found"))
		return
	}

	if h.thumbnailCandidateRepo != nil && entry.ThumbnailSource == model.ThumbnailSourceUserSelected {
		candidates, listErr := h.thumbnailCandidateRepo.ListByEntryID(c.Request.Context(), entryID)
		if listErr == nil {
			for _, cand := range candidates {
				if !cand.Selected {
					continue
				}
				// Resolve the candidate path
				sourcePath := cand.Path
				if !filepath.IsAbs(sourcePath) {
					if cand.RevisionID != nil && *cand.RevisionID != "" && h.archiveRevisionRepo != nil {
						// Look up revision to get root_path
						revision, revErr := h.archiveRevisionRepo.GetByID(c.Request.Context(), *cand.RevisionID)
						if revErr == nil && revision != nil && revision.RootPath != "" {
							sourcePath = filepath.Join(revision.RootPath, filepath.FromSlash(cand.Path))
						}
					}
				}
				if data, readErr := os.ReadFile(sourcePath); readErr == nil {
					contentType := "image/webp"
					if strings.HasSuffix(strings.ToLower(sourcePath), ".jpg") || strings.HasSuffix(strings.ToLower(sourcePath), ".jpeg") {
						contentType = "image/jpeg"
					} else if strings.HasSuffix(strings.ToLower(sourcePath), ".png") {
						contentType = "image/png"
					}
					c.Data(http.StatusOK, contentType, data)
					return
				}
			}
		}
	}

	data, contentType, err := h.fileService.GetThumbnail(entryID)
	if err != nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("thumbnail not found"))
		return
	}

	c.Data(http.StatusOK, contentType, data)
}

// GetRawThumbnail handles GET /api/v1/files/thumbnails/raw/*path
func (h *FileHandler) GetRawThumbnail(c *gin.Context) {
	raw := strings.TrimPrefix(c.Param("path"), "/")
	if raw == "" {
		c.Data(http.StatusBadRequest, "text/plain", []byte("path required"))
		return
	}
	if hasTraversalToken(raw) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}
	if once, ok := decodeOnce(raw); ok && hasTraversalToken(once) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}

	clean := filepath.Clean(raw)
	if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}

	full := filepath.Join(h.fileService.GetConfig().ThumbDir, clean)
	root := filepath.Clean(h.fileService.GetConfig().ThumbDir) + string(filepath.Separator)
	fullClean := filepath.Clean(full)
	if !strings.HasPrefix(fullClean, root) {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}

	data, err := os.ReadFile(fullClean)
	if err != nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("not found"))
		return
	}
	c.Data(http.StatusOK, "image/webp", data)
}

// DeleteArchive handles DELETE /api/v1/files/archive/:entryID
func (h *FileHandler) DeleteArchive(c *gin.Context) {
	entryID := c.Param("entryID")

	if err := h.fileService.DeleteArchive(entryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteThumbnail handles DELETE /api/v1/files/thumbnail/:entryID
func (h *FileHandler) DeleteThumbnail(c *gin.Context) {
	entryID := c.Param("entryID")

	if err := h.fileService.DeleteThumbnail(entryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.thumbnailCandidateRepo != nil {
		_ = h.thumbnailCandidateRepo.ClearSelected(c.Request.Context(), entryID)
	}

	entry, err := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if err == nil && entry != nil {
		entry.ThumbnailSource = model.ThumbnailSourceAuto
		_ = h.entryRepo.Update(c.Request.Context(), entry)
	}

	c.Status(http.StatusNoContent)
}

// ListArchives handles GET /api/v1/files/archives/:entryID
func (h *FileHandler) ListArchives(c *gin.Context) {
	if hasTraversalInRequestPath(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traversal payload"})
		return
	}

	entryID := c.Param("entryID")
	if raw := c.Param("entryID"); hasTraversalToken(raw) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traversal payload"})
		return
	}
	if once, ok := decodeOnce(entryID); ok && hasTraversalToken(once) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid traversal payload"})
		return
	}
	if !isValidArchiveID(entryID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry_id"})
		return
	}

	entry, repoErr := h.entryRepo.GetByID(c.Request.Context(), entryID)
	if repoErr != nil || entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}
	if userID := c.GetString("user_id"); userID != "" && entry.UserID != "" && entry.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	// First try entryID-based path (new format)
	files, err := h.fileService.ListArchives(entryID)

	// If no files found, check if entry has old userID-based path
	if (len(files) == 0 || err != nil) && entry.ArchivePath != "" {
		files, err = h.fileService.ListArchivesFromPath(entry.ArchivePath)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "archive not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// GetStorageStats handles GET /api/v1/files/stats
func (h *FileHandler) GetStorageStats(c *gin.Context) {
	storage := h.fileService

	c.JSON(http.StatusOK, gin.H{
		"base_dir":     storage.GetConfig().BaseDir,
		"archives_dir": storage.GetConfig().ArchivesDir,
		"thumb_dir":    storage.GetConfig().ThumbDir,
	})
}
