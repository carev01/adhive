package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carev01/adhive/internal/model"
)

func TestRewriteHTML_AttributeAwareAndRelativeResolution(t *testing.T) {
	r := NewArchiveRewriter()
	rewrites := []model.ArchiveManifestRewrite{
		{From: "https://example.com/assets/app.css", To: "assets/app.111.css"},
		{From: "https://example.com/img/bg.png", To: "assets/bg.222.png"},
		{From: "https://example.com/img/photo.jpg", To: "assets/photo.333.jpg"},
		{From: "https://example.com/img/a.jpg", To: "assets/a.444.jpg"},
		{From: "https://example.com/img/b.jpg", To: "assets/b.555.jpg"},
	}

	html := `<!doctype html>
<html>
  <head>
    <link rel="stylesheet" href="/assets/app.css">
    <style>.hero{background:url('/img/bg.png')}</style>
  </head>
  <body>
    <img src="https://example.com/img/photo.jpg" srcset="/img/a.jpg 1x, /img/b.jpg 2x">
    <script>var keep = "https://example.com/img/photo.jpg";</script>
  </body>
</html>`

	out := r.RewriteHTML(html, rewrites, "https://example.com/page")

	if !strings.Contains(out, `href="assets/app.111.css"`) {
		t.Fatalf("expected rewritten link href, got: %s", out)
	}
	if !strings.Contains(out, `url('assets/bg.222.png')`) && !strings.Contains(out, `url("assets/bg.222.png")`) {
		t.Fatalf("expected rewritten style block url, got: %s", out)
	}
	if !strings.Contains(out, `src="assets/photo.333.jpg"`) {
		t.Fatalf("expected rewritten img src, got: %s", out)
	}
	if !strings.Contains(out, `srcset="assets/a.444.jpg 1x, assets/b.555.jpg 2x"`) {
		t.Fatalf("expected rewritten srcset, got: %s", out)
	}
	// Ensure script text is not globally mutated by naive string replacement.
	if !strings.Contains(out, `var keep = "https://example.com/img/photo.jpg"`) {
		t.Fatalf("expected script content to remain unchanged, got: %s", out)
	}
}

func TestRewriteCSS_ResolvesRelativeURLAndImport(t *testing.T) {
	r := NewArchiveRewriter()
	rewrites := []model.ArchiveManifestRewrite{
		{From: "https://example.com/css/fonts.css", To: "assets/fonts.aaa.css"},
		{From: "https://example.com/fonts/f.woff2", To: "assets/f.bbb.woff2"},
	}

	css := `@import "./fonts.css";
.x{src:url('../fonts/f.woff2#iefix')}`

	out := r.RewriteCSS(css, rewrites, "https://example.com/css/main.css")

	if !strings.Contains(out, `@import "assets/fonts.aaa.css"`) {
		t.Fatalf("expected rewritten @import, got: %s", out)
	}
	if !strings.Contains(out, `url('assets/f.bbb.woff2#iefix')`) && !strings.Contains(out, `url("assets/f.bbb.woff2#iefix")`) {
		t.Fatalf("expected rewritten url(...) with fragment preserved, got: %s", out)
	}
}

func TestRewriteDownloadedCSS_RewritesInPlace(t *testing.T) {
	r := NewArchiveRewriter()
	revisionRoot := t.TempDir()
	assetsDir := filepath.Join(revisionRoot, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	cssPath := filepath.Join(assetsDir, "style.css")
	if err := os.WriteFile(cssPath, []byte(`.card{background:url('../img/a.jpg')}`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	assets := []*model.ArchiveAsset{
		{
			Kind:           model.ArchiveAssetKindCSS,
			MimeType:       "text/css",
			DownloadStatus: model.ArchiveAssetDownloadStatusOK,
			LocalPath:      "assets/style.css",
			SourceURL:      "https://example.com/css/style.css",
		},
	}
	rewrites := []model.ArchiveManifestRewrite{
		{From: "https://example.com/img/a.jpg", To: "assets/a.local.jpg"},
	}

	if err := r.RewriteDownloadedCSS(revisionRoot, assets, rewrites, "https://example.com/page"); err != nil {
		t.Fatalf("RewriteDownloadedCSS returned error: %v", err)
	}

	b, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read css: %v", err)
	}
	if !strings.Contains(string(b), `url('assets/a.local.jpg')`) && !strings.Contains(string(b), `url("assets/a.local.jpg")`) {
		t.Fatalf("expected rewritten css file, got: %s", string(b))
	}
}
