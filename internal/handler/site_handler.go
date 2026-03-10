package handler

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/carev01/adhive/internal/model"
)

// SiteHandler defines the interface for site-specific scraping handlers
type SiteHandler interface {
	// Name returns the handler name
	Name() string

	// Domains returns the list of domains this handler supports
	Domains() []string

	// CanHandle checks if this handler can handle the given URL
	CanHandle(url string) bool

	// ExtractMetadata extracts site-specific metadata from HTML
	ExtractMetadata(ctx context.Context, html, rawURL string) (*model.SiteMetadata, error)

	// Selectors returns CSS selectors for content extraction
	Selectors() *SiteSelectors
}

// SiteSelectors holds CSS selectors for content extraction
type SiteSelectors struct {
	Title       string
	Description string
	Price       string
	Location    string
	Phone       string
	Images      string
	Container   string
}

// HandlerRegistry manages all registered site handlers
type HandlerRegistry struct {
	handlers []SiteHandler
	fallback SiteHandler
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry() *HandlerRegistry {
	registry := &HandlerRegistry{
		fallback: &GenericHandler{},
	}

	// Register built-in handlers
	registry.Register(NewOLXHandler())
	registry.Register(NewMercadoLivreHandler())

	return registry
}

// Register adds a new handler to the registry
func (r *HandlerRegistry) Register(handler SiteHandler) {
	r.handlers = append(r.handlers, handler)
}

// GetHandler returns the appropriate handler for a URL
func (r *HandlerRegistry) GetHandler(rawURL string) SiteHandler {
	for _, h := range r.handlers {
		if h.CanHandle(rawURL) {
			return h
		}
	}
	return r.fallback
}

// GenericHandler is the default handler for unknown sites
type GenericHandler struct{}

func (h *GenericHandler) Name() string              { return "generic" }
func (h *GenericHandler) Domains() []string         { return nil }
func (h *GenericHandler) CanHandle(url string) bool { return true }
func (h *GenericHandler) Selectors() *SiteSelectors {
	return &SiteSelectors{
		Title:       "title",
		Description: "meta[name=description]",
		Images:      "meta[property='og:image']",
	}
}
func (h *GenericHandler) ExtractMetadata(ctx context.Context, html, rawURL string) (*model.SiteMetadata, error) {
	return &model.SiteMetadata{}, nil
}

// OLXHandler handles olx.com.br URLs
type OLXHandler struct {
	selectors *SiteSelectors
}

func NewOLXHandler() *OLXHandler {
	return &OLXHandler{
		selectors: &SiteSelectors{
			Title:       "h1[data-testid='ad-title'], h1[class*='ad__title']",
			Description: "span[data-testid='ad-description'], div[class*='ad__description']",
			Price:       "span[data-testid='ad-price'], span[class*='ad__price']",
			Location:    "span[data-testid='ad-location'], span[class*='ad__location']",
			Phone:       "a[data-testid='phone-button'], button[data-testid='phone-button']",
			Images:      "img[data-testid='ad-image'], img[class*='ad__image']",
			Container:   "div[data-testid='ad-content'], div[class*='ad__content']",
		},
	}
}

func (h *OLXHandler) Name() string { return "olx" }
func (h *OLXHandler) Domains() []string {
	return []string{"olx.com.br", "www.olx.com.br"}
}
func (h *OLXHandler) Selectors() *SiteSelectors { return h.selectors }
func (h *OLXHandler) CanHandle(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	domain := strings.ToLower(u.Hostname())
	for _, d := range h.Domains() {
		if strings.HasSuffix(domain, d) {
			return true
		}
	}
	return false
}
func (h *OLXHandler) ExtractMetadata(ctx context.Context, html, rawURL string) (*model.SiteMetadata, error) {
	// OLX-specific extraction logic
	// Will be implemented with actual HTML parsing
	return &model.SiteMetadata{
		Source: "olx",
	}, nil
}

// MercadoLivreHandler handles mercadolivre.com.br URLs
type MercadoLivreHandler struct {
	selectors *SiteSelectors
}

func NewMercadoLivreHandler() *MercadoLivreHandler {
	return &MercadoLivreHandler{
		selectors: &SiteSelectors{
			Title:       "h1[class*='ui-pdp-title'], h1[itemprop='name']",
			Description: "div[class*='ui-pdp-description'], div[itemprop='description']",
			Price:       "span[class*='price-tag'], span[itemprop='price']",
			Location:    "span[class*='ui-pdp-address'], div[itemprop='address']",
			Images:      "img[class*='ui-pdp-image'], figure img",
			Container:   "div[class*='ui-pdp-container'], div[itemtype*='Product']",
		},
	}
}

func (h *MercadoLivreHandler) Name() string { return "mercadolivre" }
func (h *MercadoLivreHandler) Domains() []string {
	return []string{"mercadolivre.com.br", "www.mercadolivre.com.br", "produto.mercadolivre.com.br"}
}
func (h *MercadoLivreHandler) Selectors() *SiteSelectors { return h.selectors }
func (h *MercadoLivreHandler) CanHandle(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	domain := strings.ToLower(u.Hostname())
	for _, d := range h.Domains() {
		if strings.HasSuffix(domain, d) {
			return true
		}
	}
	return false
}
func (h *MercadoLivreHandler) ExtractMetadata(ctx context.Context, html, rawURL string) (*model.SiteMetadata, error) {
	return &model.SiteMetadata{
		Source: "mercadolivre",
	}, nil
}

// PhoneRegex extracts Brazilian phone numbers
var PhoneRegex = regexp.MustCompile(`(?:(?:\+?55\s?)?(?:\(?\d{2}\)?\s?)?(?:9\d{4}|\d{4,5})-?\d{4})`)

// ExtractPhoneNumbers extracts phone numbers from text
func ExtractPhoneNumbers(text string) []string {
	matches := PhoneRegex.FindAllString(text, -1)
	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		cleaned := strings.ReplaceAll(m, " ", "")
		cleaned = strings.ReplaceAll(cleaned, "-", "")
		cleaned = strings.ReplaceAll(cleaned, "(", "")
		cleaned = strings.ReplaceAll(cleaned, ")", "")
		if !seen[cleaned] {
			seen[cleaned] = true
			result = append(result, cleaned)
		}
	}
	return result
}
