package service

import (
	"testing"
)

func TestExtractPhone_BrazilianFormats(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "phone with dashes",
			html:     `<html><body>Call me at (11) 91234-5678</body></html>`,
			expected: "(11) 91234-5678",
		},
		{
			name:     "phone without dashes",
			html:     `<html><body>Call me at 11 91234 5678</body></html>`,
			expected: "11 91234 5678",
		},
		{
			name:     "phone with country code",
			html:     `<html><body>+55 (11) 91234-5678</body></html>`,
			expected: "(11) 91234-5678",
		},
		{
			name:     "tel link",
			html:     `<html><body><a href="tel:+5511912345678">Call</a></body></html>`,
			expected: "+5511912345678",
		},
		{
			name:     "whatsapp mention",
			html:     `<html><body>Whatsapp: (11) 99999-9999</body></html>`,
			expected: "(11) 99999-9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.PhoneNumber != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, m.PhoneNumber)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "og:title takes precedence",
			html:     `<html><head><title>Page Title</title><meta property="og:title" content="OG Title"></head></html>`,
			expected: "OG Title",
		},
		{
			name:     "falls back to title tag",
			html:     `<html><head><title>Page Title</title></head></html>`,
			expected: "Page Title",
		},
		{
			name:     "empty html",
			html:     `<html><head></head></html>`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Title != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, m.Title)
			}
		})
	}
}

func TestExtractDescription(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "og:description takes precedence",
			html:     `<html><head><meta name="description" content="Meta Desc"><meta property="og:description" content="OG Desc"></head></html>`,
			expected: "OG Desc",
		},
		{
			name:     "falls back to meta description",
			html:     `<html><head><meta name="description" content="Meta Desc"></head></html>`,
			expected: "Meta Desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Description != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, m.Description)
			}
		})
	}
}

// Location extraction is disabled - user manually manages location
// These tests verify that location is NOT extracted
func TestExtractLocation_Disabled(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name string
		html string
	}{
		{
			name: "itemprop address",
			html: `<html><body><span itemprop="address">São Paulo, SP</span></body></html>`,
		},
		{
			name: "meta geo placename",
			html: `<html><head><meta name="geo.placename" content="Rio de Janeiro"><meta name="geo.region" content="RJ"></head></html>`,
		},
		{
			name: "location class",
			html: `<html><body><div class="location">Belo Horizonte, MG</div></body></html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Location extraction is disabled - should be empty
			if m.Location != "" {
				t.Errorf("expected empty location (extraction disabled), got %q", m.Location)
			}
		})
	}
}

func TestExtractEmail(t *testing.T) {
	extractor := NewMetadataExtractor()

	html := `<html><body>Contact: test@example.com</body></html>`
	m, err := extractor.Extract(html, "http://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Email != "test@example.com" {
		t.Errorf("expected test@example.com, got %q", m.Email)
	}
}

func TestExtractImages(t *testing.T) {
	extractor := NewMetadataExtractor()

	html := `<html><head><meta property="og:image" content="/images/product.jpg"></head></html>`
	m, err := extractor.Extract(html, "http://example.com/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Images) != 1 || m.Images[0] != "http://example.com/images/product.jpg" {
		t.Errorf("expected resolved image URL, got %v", m.Images)
	}
}

func TestExtractPrice(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "class price",
			html:     `<html><body><div class="price">R$ 2.000,00</div></body></html>`,
			expected: "R$ 2.000,00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Price != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, m.Price)
			}
		})
	}
}

func TestToJSON(t *testing.T) {
	m := &Metadata{
		Title:       "Test Title",
		Description: "Test Description",
		PhoneNumber: "(11) 91234-5678",
	}

	jsonData, err := m.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"title":"Test Title","description":"Test Description","phone_number":"(11) 91234-5678"}`
	if string(jsonData) != expected {
		t.Errorf("expected %s, got %s", expected, string(jsonData))
	}
}

func TestMetadata_MissingFields(t *testing.T) {
	// Test that missing fields don't cause errors
	extractor := NewMetadataExtractor()
	html := `<html><head><title>Minimal</title></head><body>No metadata here</body></html>`

	m, err := extractor.Extract(html, "http://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Title should be found
	if m.Title != "Minimal" {
		t.Errorf("expected title 'Minimal', got %q", m.Title)
	}

	// Other fields should be empty, not error
	if m.Description != "" {
		t.Errorf("expected empty description, got %q", m.Description)
	}
	if m.PhoneNumber != "" {
		t.Errorf("expected empty phone, got %q", m.PhoneNumber)
	}
	if m.Location != "" {
		t.Errorf("expected empty location, got %q", m.Location)
	}
}

// Location extraction is disabled - user manually manages location
// These tests verify that location is NOT extracted even with complex HTML
func TestExtractLocation_CleansNewlinesAndWhitespace_Disabled(t *testing.T) {
	extractor := NewMetadataExtractor()

	tests := []struct {
		name string
		html string
	}{
		{
			name: "location with nested spans and newlines",
			html: `<html><body>
				<div class="location">
					<span class="city">São Paulo</span>, 
					<span class="state">SP</span>
				</div>
			</body></html>`,
		},
		{
			name: "location with links and extra whitespace",
			html: `<html><body>
				<div class="location">
					<a href="/city">Rio de Janeiro</a> - 
					<span class="neighborhood">Centro</span>
				</div>
			</body></html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := extractor.Extract(tt.html, "http://example.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Location extraction is disabled - should be empty
			if m.Location != "" {
				t.Errorf("expected empty location (extraction disabled), got %q", m.Location)
			}
		})
	}
}
