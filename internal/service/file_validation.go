package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
)

// Max file sizes
const (
	MaxImageSize   = 10 * 1024 * 1024 // 10MB
	MaxArchiveSize = 100 * 1024 * 1024 // 100MB
)

// Allowed MIME types
var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

var allowedArchiveMIME = map[string]bool{
	"application/zip":               true,
	"application/x-zip-compressed":  true,
	"application/gzip":              true,
	"application/x-gzip":            true,
}

// Magic byte signatures for common file types
var magicBytes = map[string][]byte{
	"image/jpeg": {0xFF, 0xD8, 0xFF},
	"image/png":  {0x89, 0x50, 0x4E, 0x47},
	"image/gif":  {0x47, 0x49, 0x46, 0x38},
	"image/webp": {0x52, 0x49, 0x46, 0x46}, // RIFF....WEBP
	"application/zip":               {0x50, 0x4B, 0x03, 0x04},
	"application/gzip":              {0x1F, 0x8B},
}

// FileValidator validates uploaded files
type FileValidator struct {
	maxSize     int64
	allowedMIME map[string]bool
	magicBytes  map[string][]byte
}

// ValidationResult holds the result of file validation
type ValidationResult struct {
	Valid       bool
	ContentType string
	FileSize    int64
	Error       error
}

// NewImageValidator creates a validator for image uploads
func NewImageValidator() *FileValidator {
	return &FileValidator{
		maxSize:     MaxImageSize,
		allowedMIME: allowedImageMIME,
		magicBytes:  magicBytes,
	}
}

// NewArchiveValidator creates a validator for archive uploads
func NewArchiveValidator() *FileValidator {
	return &FileValidator{
		maxSize:     MaxArchiveSize,
		allowedMIME: allowedArchiveMIME,
		magicBytes:  magicBytes,
	}
}

// ValidateFile validates a file from a multipart form
func (v *FileValidator) ValidateFile(file *multipart.FileHeader) (*ValidationResult, error) {
	// Check file size
	if file.Size > v.maxSize {
		return &ValidationResult{
			Valid:    false,
			FileSize: file.Size,
			Error:    fmt.Errorf("file size exceeds maximum of %d bytes", v.maxSize),
		}, nil
	}

	// Open the file
	f, err := file.Open()
	if err != nil {
		return &ValidationResult{
			Valid:    false,
			FileSize: file.Size,
			Error:    fmt.Errorf("failed to open file: %w", err),
		}, nil
	}
	defer f.Close()

	// Read first bytes for magic byte check
	header := make([]byte, 12)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return &ValidationResult{
			Valid:    false,
			FileSize: file.Size,
			Error:    fmt.Errorf("failed to read file header: %w", err),
		}, nil
	}

	// Check magic bytes
	contentType := v.detectContentType(header[:n])
	if contentType == "" {
		return &ValidationResult{
			Valid:       false,
			FileSize:    file.Size,
			ContentType: "unknown",
			Error:       fmt.Errorf("unrecognized file type"),
		}, nil
	}

	// Check MIME type
	if !v.allowedMIME[contentType] {
		return &ValidationResult{
			Valid:       false,
			FileSize:    file.Size,
			ContentType: contentType,
			Error:       fmt.Errorf("file type %s not allowed", contentType),
		}, nil
	}

	return &ValidationResult{
		Valid:       true,
		ContentType: contentType,
		FileSize:    file.Size,
		Error:       nil,
	}, nil
}

// detectContentType detects content type from magic bytes
func (v *FileValidator) detectContentType(header []byte) string {
	for mimeType, magic := range v.magicBytes {
		if len(header) >= len(magic) {
			match := true
			for i, b := range magic {
				if header[i] != b {
					match = false
					break
				}
			}
			if match {
				return mimeType
			}
		}
	}
	return ""
}

// SanitizeFilename removes potentially dangerous characters from filename
func SanitizeFilename(filename string) string {
	// Get just the filename, not the full path
	filename = filepath.Base(filename)

	// Replace spaces with underscores
	filename = strings.ReplaceAll(filename, " ", "_")

	// Remove any path traversal attempts
	filename = strings.ReplaceAll(filename, "..", "")

	// Remove characters that could be used for injection
	// Keep only alphanumeric, dash, underscore, dot
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_\.]`)
	filename = reg.ReplaceAllString(filename, "")

	// Limit length
	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		name := filename[:len(filename)-len(ext)]
		name = name[:255-len(ext)]
		filename = name + ext
	}

	// If filename is empty after sanitization, generate a default
	if filename == "" || filename == "." {
		filename = "file"
	}

	return filename
}

// ValidateImageFile is a convenience function for validating image files
func ValidateImageFile(file *multipart.FileHeader) (*ValidationResult, error) {
	return NewImageValidator().ValidateFile(file)
}