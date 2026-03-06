package service

import (
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// SelectorGroup defines a named group of selectors with a description
// for auditing why elements were removed
type SelectorGroup struct {
	Name      string
	Selectors []string
}

// ConsentStripper removes age verification and consent popups from archived HTML.
// It uses compiled CSS selectors for correct, efficient matching.
type ConsentStripper struct {
	matchers []compiledGroup
}

type compiledGroup struct {
	name     string
	matchers []cascadia.Selector
}

// DefaultSelectorGroups returns selector groups for common consent/age patterns.
// Selectors are deliberately specific to avoid removing legitimate content.
func DefaultSelectorGroups() []SelectorGroup {
	return []SelectorGroup{
		{
			Name: "age-gates",
			Selectors: []string{
				// Specific known IDs — safe, highly targeted
				"#a31-agegate",
				"#modalAdulto",
				"#formulario_adulto",
				`div[id*="agegate"]`,
				`div[class*="agegate"]`,
				`div[class*="age-gate"]`,
				`div[class*="age-verification"]`,
				// fatalmodel.com — "Conteúdo adulto" modal with "Concordo" button
				`div.fm-new-visitor-modal`,
			},
		},
		{
			Name: "cookie-consent",
			Selectors: []string{
				"#capa_configuracion_cookies",
				`div[id*="cookie-consent"]`,
				`div[id*="cookie-banner"]`,
				`div[class*="cookie-banner"]`,
				`div[class*="cookie-consent"]`,
				`div[class*="lgpd-banner"]`,
				`div[class*="consent-banner"]`,
				`div[class*="consent-modal"]`,
			},
		},
		{
			Name: "site-specific-overlays",
			Selectors: []string{
				// Only target overlays with known consent-related siblings/structure
				"div.geolocation-overlay",
				"div.second-overlay",
				// NOT ".overlay" alone — too generic
			},
		},
	}
}

// NewConsentStripper compiles selectors once for repeated use.
// Returns an error if any selector is invalid (catch at startup, not runtime).
func NewConsentStripper(groups []SelectorGroup) (*ConsentStripper, error) {
	compiled := make([]compiledGroup, 0, len(groups))

	for _, g := range groups {
		cg := compiledGroup{name: g.Name}
		for _, sel := range g.Selectors {
			matcher, err := cascadia.Compile(sel)
			if err != nil {
				return nil, fmt.Errorf(
					"invalid selector %q in group %q: %w",
					sel, g.Name, err,
				)
			}
			cg.matchers = append(cg.matchers, matcher)
		}
		compiled = append(compiled, cg)
	}

	return &ConsentStripper{matchers: compiled}, nil
}

// Strip removes consent/age gate elements from HTML content.
// Returns the cleaned HTML and the count of removed elements.
func (s *ConsentStripper) Strip(htmlContent string) (string, int, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", 0, fmt.Errorf("parse html: %w", err)
	}

	// Phase 1: Collect all matching nodes (don't mutate during traversal)
	var toRemove []*html.Node
	for _, group := range s.matchers {
		for _, matcher := range group.matchers {
			// cascadia.QueryAll does correct, complete tree traversal
			matches := cascadia.QueryAll(doc, matcher)
			toRemove = append(toRemove, matches...)
		}
	}

	// Phase 2: Deduplicate — don't remove a node whose ancestor is
	// already being removed (would panic on nil Parent)
	toRemove = s.filterAncestors(toRemove)

	// Phase 3: Remove collected nodes (safe — no concurrent traversal)
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	var sb strings.Builder
	if err := html.Render(&sb, doc); err != nil {
		return "", 0, fmt.Errorf("render html: %w", err)
	}

	return sb.String(), len(toRemove), nil
}

// filterAncestors removes nodes from the list whose parent/ancestor
// is also in the list, preventing double-removal panics.
func (s *ConsentStripper) filterAncestors(nodes []*html.Node) []*html.Node {
	// Build a set for O(1) lookup
	removing := make(map[*html.Node]struct{}, len(nodes))
	for _, n := range nodes {
		removing[n] = struct{}{}
	}

	filtered := make([]*html.Node, 0, len(nodes))
	for _, n := range nodes {
		if !s.hasAncestorIn(n, removing) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func (s *ConsentStripper) hasAncestorIn(
	n *html.Node,
	set map[*html.Node]struct{},
) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if _, ok := set[p]; ok {
			return true
		}
	}
	return false
}