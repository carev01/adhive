package service

import (
	"testing"
)

func TestThumbnailService_NewThumbnailService(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	if svc.dataDir != "/data" {
		t.Errorf("expected dataDir /data, got %s", svc.dataDir)
	}
	if svc.maxWidth != 300 {
		t.Errorf("expected maxWidth 300, got %d", svc.maxWidth)
	}
	if svc.maxHeight != 300 {
		t.Errorf("expected maxHeight 300, got %d", svc.maxHeight)
	}
}

func TestExtractImageURL_OGImage(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	html := `<html><head><meta property="og:image" content="https://example.com/image.jpg"></head></html>`
	
	result := svc.extractImageURL(html, "https://example.com/page")
	if result != "https://example.com/image.jpg" {
		t.Errorf("expected og:image URL, got %s", result)
	}
}

func TestExtractImageURL_TwitterImage(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	html := `<html><head><meta name="twitter:image" content="https://example.com/twitter.jpg"></head></html>`
	
	result := svc.extractImageURL(html, "https://example.com/page")
	if result != "https://example.com/twitter.jpg" {
		t.Errorf("expected twitter:image URL, got %s", result)
	}
}

func TestExtractImageURL_FirstImage(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	html := `<html><body><img src="https://example.com/product.jpg"></body></html>`
	
	result := svc.extractImageURL(html, "https://example.com/page")
	if result != "https://example.com/product.jpg" {
		t.Errorf("expected first image URL, got %s", result)
	}
}

func TestExtractImageURL_PrefersOG(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	html := `<html>
		<head>
			<meta property="og:image" content="https://example.com/og.jpg">
			<meta name="twitter:image" content="https://example.com/twitter.jpg">
		</head>
		<body><img src="https://example.com/first.jpg"></body>
	</html>`
	
	result := svc.extractImageURL(html, "https://example.com/page")
	if result != "https://example.com/og.jpg" {
		t.Errorf("expected og:image to take precedence, got %s", result)
	}
}

func TestResolveURL_Absolute(t *testing.T) {
	svc := NewThumbnailService("/data")
	
	tests := []struct {
		src   string
		base  string
		want  string
	}{
		{"https://other.com/img.jpg", "https://example.com/page", "https://other.com/img.jpg"},
		{"/images/logo.png", "https://example.com/path/page", "https://example.com/images/logo.png"},
		{"logo.png", "https://example.com/path/page", "https://example.com/path/logo.png"},
	}
	
	for _, tt := range tests {
		got := svc.resolveURL(tt.src, tt.base)
		if got != tt.want {
			t.Errorf("resolveURL(%s, %s) = %s; want %s", tt.src, tt.base, got, tt.want)
		}
	}
}

func TestExtractMetaContent_OGImage(t *testing.T) {
	html := `<html><head><meta property="og:image" content="https://example.com/og.jpg"></head></html>`
	
	result := extractMetaContent(html, "og:image")
	if result != "https://example.com/og.jpg" {
		t.Errorf("expected og:image content, got %s", result)
	}
}

func TestExtractMetaContent_TwitterImage(t *testing.T) {
	html := `<html><head><meta name="twitter:image" content="https://example.com/twitter.jpg"></head></html>`
	
	result := extractMetaContent(html, "twitter:image")
	if result != "https://example.com/twitter.jpg" {
		t.Errorf("expected twitter:image content, got %s", result)
	}
}

func TestExtractMetaContent_Description(t *testing.T) {
	html := `<html><head><meta name="description" content="Test description"></head></html>`
	
	result := extractMetaContent(html, "description")
	if result != "Test description" {
		t.Errorf("expected description content, got %s", result)
	}
}
