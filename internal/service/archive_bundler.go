package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/carev01/adhive/internal/model"
	"github.com/google/uuid"
)

// ArchiveBundler persists revision-oriented archive bundles and manifest metadata.
type ArchiveBundler struct {
	baseDir       	string
	maxAssetBytes 	int64
	maxTotalBytes 	int64
	httpClient    	*http.Client
	consentStripper *ConsentStripper 
}

func (b *ArchiveBundler) withCaptureCookies(req *http.Request, capture *PlaywrightResult, targetURL string) {
	if req == nil || capture == nil || len(capture.Cookies) == 0 {
		return
	}

	target, err := url.Parse(targetURL)
	if err != nil || target.Hostname() == "" {
		return
	}

	host := strings.ToLower(target.Hostname())
	pairs := make([]string, 0, len(capture.Cookies))
	seen := make(map[string]struct{}, len(capture.Cookies))

	for _, cookie := range capture.Cookies {
		name := strings.TrimSpace(cookie.Name)
		value := cookie.Value
		if name == "" || value == "" {
			continue
		}

		domain := strings.TrimSpace(strings.ToLower(cookie.Domain))
		if domain == "" {
			domain = host
		}
		domain = strings.TrimPrefix(domain, ".")

		if domain != host && !strings.HasSuffix(host, "."+domain) {
			continue
		}

		path := strings.TrimSpace(cookie.Path)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(target.EscapedPath(), path) && path != "/" {
			continue
		}

		key := name + "|" + domain + "|" + path
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pairs = append(pairs, name+"="+value)
	}

	if len(pairs) > 0 {
		req.Header.Set("Cookie", strings.Join(pairs, "; "))
	}
}

func NewArchiveBundler(baseDir string) (*ArchiveBundler, error) {
    stripper, err := NewConsentStripper(DefaultSelectorGroups())
    if err != nil {
        return nil, fmt.Errorf("init consent stripper: %w", err)
    }

    return &ArchiveBundler{
        baseDir:       baseDir,
        maxAssetBytes: 8 * 1024 * 1024,
        maxTotalBytes: 64 * 1024 * 1024,
        httpClient: &http.Client{
            Timeout: 15 * time.Second,
            CheckRedirect: func(req *http.Request, via []*http.Request) error {
                return http.ErrUseLastResponse
            },
        },
        consentStripper: stripper,
    }, nil
}

// BundleInput is the data needed to build a new archive revision bundle.
type BundleInput struct {
	EntryID      string
	RevisionNo   int
	Engine       model.ArchiveEngine
	SourceURL    string
	CapturedAt   time.Time
	Capture      *PlaywrightResult
	RewriteLocal bool
}

// BundleResult is returned after a bundle is written.
type BundleResult struct {
	Revision *model.ArchiveRevision
	Assets   []*model.ArchiveAsset
	Manifest *model.ArchiveManifest
}

func (b *ArchiveBundler) Bundle(ctx context.Context, in BundleInput) (*BundleResult, error) {
	if in.Capture == nil {
		return nil, fmt.Errorf("capture result is required")
	}
	if in.EntryID == "" {
		return nil, fmt.Errorf("entry_id is required")
	}
	if in.RevisionNo <= 0 {
		return nil, fmt.Errorf("revision_no must be > 0")
	}
	if in.CapturedAt.IsZero() {
		in.CapturedAt = time.Now().UTC()
	}
	if in.Engine == "" {
		in.Engine = model.ArchiveEnginePlaywright
	}
	if !in.RewriteLocal {
		in.RewriteLocal = true
	}

	revisionID := uuid.New().String()
	revisionRoot := filepath.Join(b.baseDir, "archives", in.EntryID, fmt.Sprintf("rev-%04d", in.RevisionNo))
	assetsRoot := filepath.Join(revisionRoot, "assets")
	if err := os.MkdirAll(assetsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir revision assets: %w", err)
	}

	var assets []*model.ArchiveAsset
	var files []model.ArchiveManifestFile
	var rewrites []model.ArchiveManifestRewrite
	rewriteIndex := make(map[string]struct{})
	addRewrite := func(from, to string) {
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from == "" || to == "" {
			return
		}
		if _, exists := rewriteIndex[from]; exists {
			return
		}
		rewriteIndex[from] = struct{}{}
		rewrites = append(rewrites, model.ArchiveManifestRewrite{From: from, To: to})
	}
	var totalBytes int64
	baseURL := fallbackURL(in.Capture.FinalURL, in.SourceURL)
	normalizedURLs := dedupe(append([]string{}, in.Capture.ResourceURLs...))
	normalizedURLs = dedupe(append(normalizedURLs, in.Capture.DOMAssetURLs...))

	for _, src := range normalizedURLs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if strings.TrimSpace(src) == "" {
			continue
		}
		if isTrackerLike(src) {
			files = append(files, model.ArchiveManifestFile{
				SourceURL:      src,
				DownloadStatus: model.ArchiveAssetDownloadStatusSkipped,
				Kind:           classifyAssetKind(src, ""),
			})
			continue
		}

		localRel, mime, bytes, hash, status := b.persistAssetFromURL(ctx, src, assetsRoot, in.Capture)
		totalBytes += bytes
		if totalBytes > b.maxTotalBytes {
			status = model.ArchiveAssetDownloadStatusSkipped
			localRel = ""
			mime = ""
			bytes = 0
			hash = ""
		}

		asset := &model.ArchiveAsset{
			ID:             uuid.New().String(),
			RevisionID:     revisionID,
			RootPath:       revisionRoot,
			SourceURL:      src,
			LocalPath:      localRel,
			ContentHash:    hash,
			MimeType:       mime,
			Bytes:          bytes,
			Kind:           classifyAssetKind(src, mime),
			DownloadStatus: status,
		}
		assets = append(assets, asset)

		files = append(files, model.ArchiveManifestFile{
			SourceURL:      src,
			LocalPath:      localRel,
			ContentHash:    hash,
			MimeType:       mime,
			Bytes:          bytes,
			Kind:           asset.Kind,
			DownloadStatus: status,
		})
		if localRel != "" {
			for _, from := range buildRewriteAliases(baseURL, src) {
				addRewrite(from, localRel)
			}
		}
	}

	if in.RewriteLocal {
		rewriter := NewArchiveRewriter()
		in.Capture.HTML = rewriter.RewriteHTML(in.Capture.HTML, rewrites, baseURL)
		if err := rewriter.RewriteDownloadedCSS(revisionRoot, assets, rewrites, baseURL); err != nil {
			fmt.Printf("[archive_bundler] css rewrite warning: %v\n", err)
		}
	}

	// Strip consent/age gate overlays from the final HTML.
	// This runs AFTER URL rewriting so selectors match the rewritten DOM,
	// and BEFORE writing index.html so the file on disk is clean.
	if cleanHTML, removed, err := b.consentStripper.Strip(in.Capture.HTML); err != nil {
		// Non-fatal: archive is still usable with overlays present
		fmt.Printf("[archive_bundler] consent strip warning: %v\n", err)
	} else {
		if removed > 0 {
			fmt.Printf("[archive_bundler] stripped %d consent/overlay elements\n", removed)
		}
		in.Capture.HTML = cleanHTML
	}

	indexPath := filepath.Join(revisionRoot, "index.html")
	if err := os.WriteFile(indexPath, []byte(in.Capture.HTML), 0o644); err != nil {
		return nil, fmt.Errorf("write index html: %w", err)
	}

	stats := buildStats(files)
	manifest := &model.ArchiveManifest{
		SchemaVersion: "1.0",
		RevisionID:    revisionID,
		EntryID:       in.EntryID,
		RevisionNo:    in.RevisionNo,
		CapturedAt:    in.CapturedAt,
		Engine:        in.Engine,
		BaseURL:       baseURL,
		Status:        manifestStatus(in.Capture, stats),
		FailureReason: in.Capture.Error,
		Stats:         stats,
		Diagnostics: model.ArchiveManifestDiag{
			FinalURL:          in.Capture.FinalURL,
			HTTPStatus:        in.Capture.StatusCode,
			RedirectChain:     in.Capture.RedirectChain,
			ChallengeDetected: in.Capture.ChallengeDetected,
			TimeoutStage:      in.Capture.TimeoutStage,
			ErrorType:         in.Capture.ErrorType,
		},
		Files:    files,
		Rewrites: rewrites,
	}

	manifestPath := filepath.Join(revisionRoot, "manifest.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	revision := &model.ArchiveRevision{
		ID:           revisionID,
		EntryID:      in.EntryID,
		RevisionNo:   in.RevisionNo,
		Engine:       in.Engine,
		RootPath:     revisionRoot,
		IndexPath:    indexPath,
		ManifestPath: manifestPath,
		Status:       manifest.Status,
		CapturedAt:   in.CapturedAt,
	}
	if in.Capture.Error != "" {
		reason := in.Capture.Error
		revision.FailureReason = &reason
	}

	return &BundleResult{Revision: revision, Assets: assets, Manifest: manifest}, nil
}

func (b *ArchiveBundler) persistAssetFromURL(ctx context.Context, rawURL, assetsRoot string, capture *PlaywrightResult) (localRel, mime string, bytes int64, hash string, status model.ArchiveAssetDownloadStatus) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusError
	}

	// Skip non-HTTP(S) schemes for now
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusSkipped
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusError
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	b.withCaptureCookies(req, capture, rawURL)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusError
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusMissing
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, b.maxAssetBytes+1))
	if err != nil {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusError
	}

	if int64(len(content)) > b.maxAssetBytes {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusSkipped
	}

	sum := sha256.Sum256(content)
	hash = hex.EncodeToString(sum[:])

	// Determine filename from URL path or Content-Disposition
	filename := filepath.Base(u.Path)
	if filename == "" || filename == "/" || strings.ContainsAny(filename, "?#") {
		filename = "asset.bin"
	}
	filename = sanitizeAssetFilename(filename)

	// Add extension based on detected content type if missing
	ext := filepath.Ext(filename)
	if ext == "" {
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			ext = mimeToExt(ct)
		}
	}
	if ext == "" {
		ext = ".bin"
	}

	// Ensure unique filename using hash prefix
	name := hash[:16] + ext
	abs := filepath.Join(assetsRoot, name)
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return "", "", 0, "", model.ArchiveAssetDownloadStatusError
	}

	mime = resp.Header.Get("Content-Type")

	// Strip parameters ("; charset=utf-8", etc.) for reliable comparison
	baseMime := mime
	if idx := strings.IndexByte(baseMime, ';'); idx != -1 {
		baseMime = strings.TrimSpace(baseMime[:idx])
	}

	// Only attempt detection when the server gave us nothing useful
	if baseMime == "" || baseMime == "application/octet-stream" {
		if detected := detectMimeFromExt(ext); detected != "" {
			mime = detected 
		} else if len(content) > 0 {
			detected := http.DetectContentType(content)
			if detected != "application/octet-stream" {
				mime = detected
			}
		}
	}
	
	return filepath.ToSlash(filepath.Join("assets", name)), mime, int64(len(content)), hash, model.ArchiveAssetDownloadStatusOK
}

func buildRewriteAliases(pageBaseURL, sourceURL string) []string {
	seen := make(map[string]struct{})
	aliases := make([]string, 0, 8)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		aliases = append(aliases, v)
	}

	add(sourceURL)

	srcURL, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || !srcURL.IsAbs() {
		return aliases
	}
	srcURL.Fragment = ""
	add(srcURL.String())

	baseURL, err := url.Parse(strings.TrimSpace(pageBaseURL))
	if err != nil || !baseURL.IsAbs() {
		return aliases
	}
	if !strings.EqualFold(baseURL.Scheme, srcURL.Scheme) || !strings.EqualFold(baseURL.Host, srcURL.Host) {
		return aliases
	}

	rootPath := srcURL.EscapedPath()
	if rootPath == "" {
		rootPath = "/"
	}
	if srcURL.RawQuery != "" {
		add(rootPath + "?" + srcURL.RawQuery)
	} else {
		add(rootPath)
	}

	basePath := baseURL.EscapedPath()
	if basePath == "" {
		basePath = "/"
	}
	rel, err := filepath.Rel(path.Dir(basePath), rootPath)
	if err == nil && rel != "" {
		relWithQuery := rel
		if srcURL.RawQuery != "" {
			relWithQuery += "?" + srcURL.RawQuery
		}
		add(filepath.ToSlash(relWithQuery))
		if !strings.HasPrefix(relWithQuery, "../") && !strings.HasPrefix(relWithQuery, "./") && !strings.HasPrefix(relWithQuery, "/") {
			add("./" + filepath.ToSlash(relWithQuery))
		}
	}

	return aliases
}

// detectMimeFromExt returns a MIME type for the given file extension,
// or "" if the type cannot be determined.
func detectMimeFromExt(ext string) string {
    // Tier 1: Hardcoded overrides for types that are commonly
    // wrong or missing in OS MIME databases (e.g., Alpine containers)
    switch strings.ToLower(ext) {
    case ".css":
        return "text/css"
    case ".js", ".mjs":
        return "application/javascript"
    case ".json":
        return "application/json"
    case ".html", ".htm":
        return "text/html"
    case ".xml":
        return "application/xml"
    case ".svg":
        return "image/svg+xml"
    case ".woff":
        return "font/woff"
    case ".woff2":
        return "font/woff2"
    case ".ttf":
        return "font/ttf"
    case ".otf":
        return "font/otf"
    case ".wasm":
        return "application/wasm"
    }

    // Tier 2: OS MIME database (covers .png, .jpg, .gif, .pdf,
    // .mp4, .zip, and hundreds of others)
    if detected := mime.TypeByExtension(strings.ToLower(ext)); detected != "" {
        // Strip parameters: mime.TypeByExtension may return
        // "text/xml; charset=utf-8"
        if idx := strings.IndexByte(detected, ';'); idx != -1 {
            detected = strings.TrimSpace(detected[:idx])
        }
        return detected
    }

    // Unknown — let the caller decide what to do
    return ""
}

func mimeToExt(mime string) string {
	mime = strings.ToLower(strings.Split(mime, ";")[0])
	switch mime {
	case "text/css":
		return ".css"
	case "application/javascript", "text/javascript":
		return ".js"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "font/woff":
		return ".woff"
	case "font/woff2":
		return ".woff2"
	case "font/ttf":
		return ".ttf"
	case "font/otf":
		return ".otf"
	case "application/json":
		return ".json"
	case "application/xml":
		return ".xml"
	case "text/html":
		return ".html"
	default:
		return ""
	}
}

func classifyAssetKind(pathOrURL, mime string) model.ArchiveAssetKind {
	p := strings.ToLower(pathOrURL + " " + mime)
	switch {
	case strings.Contains(p, ".css") || strings.Contains(p, "text/css"):
		return model.ArchiveAssetKindCSS
	case strings.Contains(p, ".js") || strings.Contains(p, "javascript"):
		return model.ArchiveAssetKindJS
	case strings.Contains(p, "image/") || strings.Contains(p, ".png") || strings.Contains(p, ".jpg") || strings.Contains(p, ".jpeg") || strings.Contains(p, ".webp") || strings.Contains(p, ".svg"):
		return model.ArchiveAssetKindIMG
	case strings.Contains(p, "font/") || strings.Contains(p, ".woff") || strings.Contains(p, ".ttf") || strings.Contains(p, ".otf"):
		return model.ArchiveAssetKindFont
	case strings.Contains(p, "video/") || strings.Contains(p, "audio/"):
		return model.ArchiveAssetKindMedia
	default:
		return model.ArchiveAssetKindOther
	}
}

func manifestStatus(capture *PlaywrightResult, stats model.ArchiveManifestStats) model.ArchiveRevisionStatus {
	if capture == nil {
		return model.ArchiveRevisionStatusFailed
	}
	// Cross-domain redirect (e.g., to Instagram) is not a block - it's a redirect
	// Mark as partial since we captured the redirect destination, not the original content
	if capture.CrossDomainRedirect {
		if stats.DownloadedAssets > 0 {
			return model.ArchiveRevisionStatusPartial
		}
		return model.ArchiveRevisionStatusSuccess
	}
	if capture.ChallengeDetected {
		return model.ArchiveRevisionStatusBlocked
	}
	if capture.Error != "" || capture.StatusCode >= 400 || capture.StatusCode == 0 {
		if stats.DownloadedAssets > 0 {
			return model.ArchiveRevisionStatusPartial
		}
		return model.ArchiveRevisionStatusFailed
	}
	if stats.ErrorAssets > 0 || stats.MissingAssets > 0 {
		return model.ArchiveRevisionStatusPartial
	}
	return model.ArchiveRevisionStatusSuccess
}

func buildStats(files []model.ArchiveManifestFile) model.ArchiveManifestStats {
	stats := model.ArchiveManifestStats{TotalAssets: len(files)}
	for _, f := range files {
		stats.TotalBytes += f.Bytes
		switch f.DownloadStatus {
		case model.ArchiveAssetDownloadStatusOK:
			stats.DownloadedAssets++
		case model.ArchiveAssetDownloadStatusMissing:
			stats.MissingAssets++
		case model.ArchiveAssetDownloadStatusSkipped:
			stats.SkippedAssets++
		case model.ArchiveAssetDownloadStatusError:
			stats.ErrorAssets++
		}
	}
	return stats
}

func sanitizeAssetFilename(v string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	return re.ReplaceAllString(v, "_")
}

func fallbackURL(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func isTrackerLike(v string) bool {
	lv := strings.ToLower(v)
	blocked := []string{"google-analytics", "doubleclick", "googletagmanager", "facebook.com/tr", "hotjar", "segment.io"}
	for _, b := range blocked {
		if strings.Contains(lv, b) {
			return true
		}
	}
	return false
}
