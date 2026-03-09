package service

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Metadata represents extracted metadata from a web page
type Metadata struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Price       string   `json:"price,omitempty"`
	PhoneNumber string   `json:"phone_number,omitempty"`
	Email       string   `json:"email,omitempty"`
	Location    string   `json:"location,omitempty"`
	Images      []string `json:"images,omitempty"`
	SiteName    string   `json:"site_name,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
}

// MetadataExtractor extracts structured metadata from HTML
type MetadataExtractor struct{}

// NewMetadataExtractor creates a new MetadataExtractor
func NewMetadataExtractor() *MetadataExtractor {
	return &MetadataExtractor{}
}

// Extract extracts metadata from HTML content
func (e *MetadataExtractor) Extract(html, pageURL string) (*Metadata, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	m := &Metadata{}

	// Extract all fields
	e.extractTitle(doc, m)
	e.extractDescription(doc, m)
	e.extractPrice(doc, m)
	e.extractPhone(doc, html, m)
	e.extractEmail(html, m)
	// Location extraction disabled - user manually manages location
	// e.extractLocation(doc, html, m)
	e.extractImages(doc, pageURL, m)
	e.extractSiteName(doc, m)
	e.extractPublishedDate(doc, m)

	return m, nil
}

func (e *MetadataExtractor) extractTitle(doc *goquery.Document, m *Metadata) {
	// Try og:title first
	title := doc.Find("meta[property='og:title']").First().AttrOr("content", "")
	if title == "" {
		// Try twitter:title
		title = doc.Find("meta[name='twitter:title']").First().AttrOr("content", "")
	}
	if title == "" {
		// Fall back to <title>
		title = doc.Find("title").First().Text()
	}
	m.Title = strings.TrimSpace(title)
}

func (e *MetadataExtractor) extractDescription(doc *goquery.Document, m *Metadata) {
	// Try og:description
	desc := doc.Find("meta[property='og:description']").First().AttrOr("content", "")
	if desc == "" {
		// Try twitter:description
		desc = doc.Find("meta[name='twitter:description']").First().AttrOr("content", "")
	}
	if desc == "" {
		// Try meta description
		desc = doc.Find("meta[name='description']").First().AttrOr("content", "")
	}
	m.Description = strings.TrimSpace(desc)
}

func (e *MetadataExtractor) extractPrice(doc *goquery.Document, m *Metadata) {
	// Common price selectors for classifieds
	priceSelectors := []string{
		"[itemprop='price']",
		".price",
		".product-price",
		".offer-price",
		"[class*='price']",
		"[id*='price']",
	}

	for _, sel := range priceSelectors {
		price := doc.Find(sel).First().AttrOr("content", "")
		if price == "" {
			price = doc.Find(sel).First().Text()
		}
		if price = strings.TrimSpace(price); price != "" {
			m.Price = price
			return
		}
	}
}

func (e *MetadataExtractor) extractPhone(doc *goquery.Document, html string, m *Metadata) {
	// Try tel: links
	phone := doc.Find("a[href^='tel:']").First().AttrOr("href", "")
	if phone != "" {
		m.PhoneNumber = strings.TrimPrefix(phone, "tel:")
		return
	}

	// Try phone input fields
	phone = doc.Find("input[type='tel']").First().AttrOr("value", "")
	if phone != "" {
		m.PhoneNumber = phone
		return
	}

	// Regex for common phone patterns in HTML
	phonePatterns := []string{
		`\(\d{2,3}\)\s*\d{4,5}[-\s]*\d{4}`,
		`\+\d{1,3}\s*\(\d{2,3}\)\s*\d{4,5}[-\s]*\d{4}`,
		`\d{2,3}\s?\d{4,5}\s?\d{4}`,
		`(?:whatsapp|whatsapp:)\s*[\d\s\-\+\(\)]+`,
	}

	for _, pattern := range phonePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 0 {
			m.PhoneNumber = strings.TrimSpace(matches[0])
			return
		}
	}
}

func (e *MetadataExtractor) extractEmail(html string, m *Metadata) {
	// Email regex
	emailRe := regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	matches := emailRe.FindStringSubmatch(html)
	if len(matches) > 0 {
		m.Email = matches[0]
	}
}

func (e *MetadataExtractor) extractImages(doc *goquery.Document, pageURL string, m *Metadata) {
	var images []string

	// Try og:image
	doc.Find("meta[property='og:image']").Each(func(i int, s *goquery.Selection) {
		if src := s.AttrOr("content", ""); src != "" {
			images = append(images, e.resolveURL(src, pageURL))
		}
	})

	// Try twitter:image
	if len(images) == 0 {
		doc.Find("meta[name='twitter:image']").Each(func(i int, s *goquery.Selection) {
			if src := s.AttrOr("content", ""); src != "" {
				images = append(images, e.resolveURL(src, pageURL))
			}
		})
	}

	// Try img tags with product/item class
	if len(images) == 0 {
		doc.Find("img[class*='product'], img[class*='item'], img[class*='photo']").Each(func(i int, s *goquery.Selection) {
			if src := s.AttrOr("src", ""); src != "" && !strings.HasPrefix(src, "data:") {
				images = append(images, e.resolveURL(src, pageURL))
			}
		})
	}

	m.Images = images
}

func (e *MetadataExtractor) extractSiteName(doc *goquery.Document, m *Metadata) {
	// Try og:site_name
	siteName := doc.Find("meta[property='og:site_name']").First().AttrOr("content", "")
	m.SiteName = strings.TrimSpace(siteName)
}

func (e *MetadataExtractor) extractPublishedDate(doc *goquery.Document, m *Metadata) {
	// Try article:published_time
	publishedAt := doc.Find("meta[property='article:published_time']").First().AttrOr("content", "")
	if publishedAt != "" {
		m.PublishedAt = publishedAt
		return
	}

	// Try datePublished schema
	publishedAt = doc.Find("[itemprop='datePublished']").First().AttrOr("content", "")
	if publishedAt == "" {
		publishedAt = doc.Find("[itemprop='datePublished']").First().Text()
	}
	m.PublishedAt = strings.TrimSpace(publishedAt)
}

func (e *MetadataExtractor) resolveURL(src, base string) string {
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

// ToJSON converts metadata to JSON
func (m *Metadata) ToJSON() (json.RawMessage, error) {
	return json.Marshal(m)
}
