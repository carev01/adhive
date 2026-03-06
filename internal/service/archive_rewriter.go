package service

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carev01/adhive/internal/model"
)

// ArchiveRewriter rewrites HTML/CSS references to local archived asset paths.
type ArchiveRewriter struct{}

func NewArchiveRewriter() *ArchiveRewriter { return &ArchiveRewriter{} }

// RewriteHTML rewrites URL-bearing HTML attributes to local paths.
//
// This is intentionally attribute-aware (instead of global string replacement)
// to avoid mutating unrelated plain/script text.
func (r *ArchiveRewriter) RewriteHTML(html string, rewrites []model.ArchiveManifestRewrite, baseURL string) string {
	if strings.TrimSpace(html) == "" || len(rewrites) == 0 {
		return html
	}

	lookup := buildRewriteLookup(rewrites)
	rewrite := func(v string) string {
		return r.rewriteSingleURL(v, lookup, baseURL)
	}

	out := html

	// Common URL-bearing attributes.
	attrDQ := regexp.MustCompile(`(?is)(\b(?:src|href|poster|data-src|data-href)\s*=\s*")(.*?)(")`)
	out = attrDQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := attrDQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + rewrite(sub[2]) + sub[3]
	})

	attrSQ := regexp.MustCompile(`(?is)(\b(?:src|href|poster|data-src|data-href)\s*=\s*')(.*?)(')`)
	out = attrSQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := attrSQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + rewrite(sub[2]) + sub[3]
	})

	// srcset attributes (double and single quote variants)
	srcsetDQ := regexp.MustCompile(`(?is)(\bsrcset\s*=\s*")(.*?)(")`)
	out = srcsetDQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := srcsetDQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + rewriteSrcset(sub[2], rewrite) + sub[3]
	})

	srcsetSQ := regexp.MustCompile(`(?is)(\bsrcset\s*=\s*')(.*?)(')`)
	out = srcsetSQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := srcsetSQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + rewriteSrcset(sub[2], rewrite) + sub[3]
	})

	// style="..." / style='...'
	styleAttrDQ := regexp.MustCompile(`(?is)(\bstyle\s*=\s*")(.*?)(")`)
	out = styleAttrDQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := styleAttrDQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + r.RewriteCSS(sub[2], rewrites, baseURL) + sub[3]
	})

	styleAttrSQ := regexp.MustCompile(`(?is)(\bstyle\s*=\s*')(.*?)(')`)
	out = styleAttrSQ.ReplaceAllStringFunc(out, func(m string) string {
		sub := styleAttrSQ.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + r.RewriteCSS(sub[2], rewrites, baseURL) + sub[3]
	})

	// Inline <style> blocks.
	styleBlock := regexp.MustCompile(`(?is)(<style[^>]*>)(.*?)(</style>)`)
	out = styleBlock.ReplaceAllStringFunc(out, func(m string) string {
		sub := styleBlock.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		return sub[1] + r.RewriteCSS(sub[2], rewrites, baseURL) + sub[3]
	})

	return out
}

// RewriteCSS rewrites url(...) and @import references in CSS content.
func (r *ArchiveRewriter) RewriteCSS(css string, rewrites []model.ArchiveManifestRewrite, baseURL string) string {
	if strings.TrimSpace(css) == "" || len(rewrites) == 0 {
		return css
	}
	lookup := buildRewriteLookup(rewrites)
	rewrite := func(v string) string {
		return r.rewriteSingleURL(v, lookup, baseURL)
	}

	// url(...) references
	urlRe := regexp.MustCompile(`(?is)url\(\s*([^\)]*?)\s*\)`)
	out := urlRe.ReplaceAllStringFunc(css, func(m string) string {
		sub := urlRe.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		inner := strings.TrimSpace(sub[1])
		quote := ""
		if len(inner) >= 2 {
			if (inner[0] == '\'' && inner[len(inner)-1] == '\'') || (inner[0] == '"' && inner[len(inner)-1] == '"') {
				quote = string(inner[0])
				inner = inner[1 : len(inner)-1]
			}
		}
		next := rewrite(inner)
		if quote == "" {
			quote = `"`
		}
		return fmt.Sprintf("url(%s%s%s)", quote, next, quote)
	})

	// @import url(...)
	importURLRe := regexp.MustCompile(`(?is)@import\s+url\(\s*([^\)]*?)\s*\)`)
	out = importURLRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := importURLRe.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		inner := strings.TrimSpace(sub[1])
		quote := ""
		if len(inner) >= 2 {
			if (inner[0] == '\'' && inner[len(inner)-1] == '\'') || (inner[0] == '"' && inner[len(inner)-1] == '"') {
				quote = string(inner[0])
				inner = inner[1 : len(inner)-1]
			}
		}
		next := rewrite(inner)
		if quote == "" {
			quote = `"`
		}
		return fmt.Sprintf("@import url(%s%s%s)", quote, next, quote)
	})

	// @import "..."
	importDQRe := regexp.MustCompile(`(?is)@import\s+"([^"]+)"`)
	out = importDQRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := importDQRe.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		raw := strings.TrimSpace(sub[1])
		next := rewrite(raw)
		return fmt.Sprintf(`@import "%s"`, next)
	})

	// @import '...'
	importSQRe := regexp.MustCompile(`(?is)@import\s+'([^']+)'`)
	out = importSQRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := importSQRe.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		raw := strings.TrimSpace(sub[1])
		next := rewrite(raw)
		return fmt.Sprintf("@import '%s'", next)
	})

	return out
}

// RewriteDownloadedCSS rewrites already-downloaded CSS files in-place.
//
// URL resolution is done against each CSS file's original SourceURL so relative
// url(...) and @import references are resolved correctly.
func (r *ArchiveRewriter) RewriteDownloadedCSS(revisionRoot string, assets []*model.ArchiveAsset, rewrites []model.ArchiveManifestRewrite, pageBaseURL string) error {
	var firstErr error
	for _, a := range assets {
		if a == nil {
			continue
		}
		if a.DownloadStatus != model.ArchiveAssetDownloadStatusOK {
			continue
		}
		if a.LocalPath == "" {
			continue
		}
		if a.Kind != model.ArchiveAssetKindCSS && !strings.Contains(strings.ToLower(a.MimeType), "text/css") && !strings.HasSuffix(strings.ToLower(a.LocalPath), ".css") {
			continue
		}

		absPath := filepath.Join(revisionRoot, filepath.FromSlash(a.LocalPath))
		cssBytes, err := os.ReadFile(absPath)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		base := strings.TrimSpace(a.SourceURL)
		if base == "" {
			base = pageBaseURL
		}
		rewritten := r.RewriteCSS(string(cssBytes), rewrites, base)
		if rewritten == string(cssBytes) {
			continue
		}
		if err := os.WriteFile(absPath, []byte(rewritten), 0o644); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *ArchiveRewriter) rewriteSingleURL(raw string, lookup map[string]string, baseURL string) string {
	raw = strings.TrimSpace(strings.Trim(raw, `"'`))
	if raw == "" {
		return raw
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(raw, "#") {
		return raw
	}

	lookupKey, fragment := splitFragment(raw)
	if to, ok := lookup[lookupKey]; ok {
		return to + fragment
	}

	resolved := resolveURL(baseURL, lookupKey)
	resolvedKey, _ := splitFragment(resolved)
	if to, ok := lookup[resolvedKey]; ok {
		return to + fragment
	}

	return raw
}

func buildRewriteLookup(rewrites []model.ArchiveManifestRewrite) map[string]string {
	lookup := make(map[string]string, len(rewrites)*2)
	for _, rw := range rewrites {
		from := strings.TrimSpace(rw.From)
		to := strings.TrimSpace(rw.To)
		if from == "" || to == "" {
			continue
		}
		lookup[from] = to
		if noFrag, _ := splitFragment(from); noFrag != from {
			lookup[noFrag] = to
		}
	}
	return lookup
}

func rewriteSrcset(srcset string, rewriteFn func(string) string) string {
	items := strings.Split(srcset, ",")
	for i, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.Fields(item)
		if len(parts) == 0 {
			continue
		}
		parts[0] = rewriteFn(parts[0])
		items[i] = strings.Join(parts, " ")
	}
	return strings.Join(items, ", ")
}

func splitFragment(u string) (withoutFragment, fragment string) {
	idx := strings.IndexByte(u, '#')
	if idx < 0 {
		return u, ""
	}
	return u[:idx], u[idx:]
}

func resolveURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		if bu, err := url.Parse(baseURL); err == nil && bu.Scheme != "" {
			return bu.Scheme + ":" + raw
		}
		return raw
	}

	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if ref.IsAbs() {
		return ref.String()
	}
	bu, err := url.Parse(baseURL)
	if err != nil || bu.Scheme == "" || bu.Host == "" {
		return raw
	}
	return bu.ResolveReference(ref).String()
}
