# ADR-001: Shiori-to-AdHive Import Architecture

**Status:** Proposed  
**Date:** 2026-03-07  
**Decision Makers:** @bumblebee, @jarvis  
**Supersedes:** N/A

---

## Context

Everton has a Shiori instance containing ~164 classified ad bookmarks that need to be migrated to AdHive. Shiori stores data in MariaDB with BoltDB archives, while AdHive uses SQLite with a file-based archive system. We need to design an import architecture that preserves all data including HTML content, thumbnails, and tags.

### Shiori Data Overview

| Component | Format | Count | Size |
|-----------|--------|-------|------|
| Bookmarks | MariaDB dump | ~164 entries | ~1.2 MB SQL |
| Archives | BoltDB files | 164 files | 1.3 GB |
| Thumbnails | JPEG images | 172 files | 8.7 MB |
| Tags | MariaDB | 8 tags | - |
| Schema | MariaDB | v0.8.5 | - |

### Shiori Schema (Source)

```sql
-- Account table (single user for this export)
account (
  id INT,
  username VARCHAR(250),
  password BINARY(80),  -- bcrypt hash
  owner TINYINT(1),
  config JSON          -- {"ShowId":true,"Theme":"dark",...}
)

-- Bookmarks (main content)
bookmark (
  id INT PRIMARY KEY,
  url TEXT NOT NULL,
  title TEXT NOT NULL,
  excerpt TEXT DEFAULT '',
  author TEXT DEFAULT '',
  public TINYINT(1) DEFAULT 0,
  content MEDIUMTEXT DEFAULT '',      -- Readability-extracted text
  html MEDIUMTEXT DEFAULT '',         -- Full HTML with assets embedded
  created_at TIMESTAMP,
  has_content TINYINT(1) DEFAULT 0,
  modified_at TIMESTAMP
)

-- Tags
tag (
  id INT PRIMARY KEY,
  name VARCHAR(250) UNIQUE
)

-- Junction table
bookmark_tag (
  bookmark_id INT,
  tag_id INT
)
```

### BoltDB Archive Structure

Each `{bookmark_id}` file in `archive/` is a BoltDB database containing:
- **Key:** `archive` → **Value:** WARC-like binary data (gzipped?)
- **Key:** `image` → **Value:** Screenshot/thumbnail data

### AdHive Schema (Target)

```go
// Main entry table
CatalogEntry {
  ID            string          // UUID
  UserID        string          // Foreign key
  URL           string
  Title         string
  Description   string
  PhoneNumber   string          // Extracted from content
  Location      string          // Extracted from content
  ThumbnailPath string
  ArchivePath   string
  ArchiveStatus ArchiveStatus   // pending/success/failed
  ArchiveFidelity ArchiveFidelity // high/partial/low
  CreatedAt     time.Time
  UpdatedAt     time.Time
}

// Archive revision tracking
ArchiveRevision {
  ID            string
  EntryID       string
  RevisionNo    int
  Engine        ArchiveEngine    // playwright/http
  RootPath      string           // File path to archive
  Status        ArchiveRevisionStatus
  CapturedAt    time.Time
}

// Individual archived assets
ArchiveAsset {
  ID             string
  RevisionID     string
  SourceURL      string
  LocalPath      string
  ContentHash    string
  MimeType       string
  Bytes          int64
  Kind           ArchiveAssetKind // css/js/img/font/media/other
  DownloadStatus ArchiveAssetDownloadStatus
}

// Thumbnail candidates
ThumbnailCandidate {
  ID         string
  EntryID    string
  RevisionID *string
  SourceType ThumbnailCandidateSourceType
  Path       string
  Score      float64
  Selected   bool
}
```

---

## Decision

We will implement a **two-phase import architecture**:

### Phase 1: SQL Data Migration (Go CLI Tool)

Create a dedicated import command in `cmd/import-shiori/` that:

1. **Parses MariaDB dump** using regex-based SQL parsing (simpler than MySQL connection)
2. **Creates AdHive entries** with UUID mapping from Shiori IDs
3. **Migrates tags** with user association
4. **Stores HTML content** in AdHive's `content` field temporarily

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PHASE 1: SQL IMPORT                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌──────────────────┐       ┌──────────────────┐       ┌────────────────┐  │
│   │  dump.sql        │       │  Import         │       │  AdHive DB     │  │
│   │  (MariaDB)       │──────►│  Parser (Go)    │──────►│  (SQLite)      │  │
│   └──────────────────┘       └──────────────────┘       └────────────────┘  │
│                                                                              │
│   Maps:                                                                      │
│   • bookmark.id → catalog_entries.id (UUID generation)                      │
│   • bookmark.url → catalog_entries.url                                      │
│   • bookmark.title → catalog_entries.title                                  │
│   • bookmark.excerpt → catalog_entries.description                          │
│   • bookmark.html → catalog_entries.content (temp)                          │
│   • bookmark.created_at → catalog_entries.created_at                        │
│   • tag.name → tags.name                                                    │
│   • bookmark_tag → entry_tags (junction)                                    │
│                                                                              │
│   ID Mapping Table (temporary):                                             │
│   ┌────────────────┐                                                        │
│   │ shiori_id → adhive_uuid │  (Stored in memory + JSON backup)             │
│   └────────────────┘                                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Phase 2: BoltDB Archive Extraction (Go CLI Tool)

A separate extraction process that:

1. **Reads BoltDB files** using `go.etcd.io/bbolt` library
2. **Extracts HTML content** from `archive` bucket
3. **Extracts images** from `image` bucket (if present)
4. **Converts to AdHive archive format** with proper asset structure
5. **Creates ArchiveRevision and ArchiveAsset records**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PHASE 2: ARCHIVE EXTRACTION                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌──────────────────┐       ┌──────────────────┐       ┌────────────────┐  │
│   │  archive/{id}     │       │  BoltDB         │       │  AdHive        │  │
│   │  (BoltDB files)   │──────►│  Extractor (Go)  │──────►│  Archive Store │  │
│   └──────────────────┘       └──────────────────┘       └────────────────┘  │
│                                                                              │
│   Process per bookmark:                                                     │
│   1. Open BoltDB file                                                        │
│   2. Read "archive" bucket → HTML content                                   │
│   3. Read "image" bucket → Thumbnail data                                   │
│   4. Parse HTML, extract asset URLs                                         │
│   5. Create archive revision: data/archives/{uuid}/rev-{ts}/                │
│   6. Store assets, rewrite URLs                                             │
│   7. Create ArchiveRevision record                                          │
│   8. Create ArchiveAsset records for each asset                             │
│   9. Update catalog_entries.archive_status = 'success'                      │
│                                                                              │
│   Output:                                                                    │
│   data/archives/{entry-uuid}/                                               │
│   ├── rev-{timestamp}/                                                      │
│   │   ├── index.html                                                        │
│   │   └── assets/                                                            │
│   │       ├── style-{hash}.css                                               │
│   │       ├── script-{hash}.js                                               │
│   │       └── img-{hash}.jpg                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Phase 3: Thumbnail Migration

Migrate Shiori thumbnails to AdHive's thumbnail candidate system:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PHASE 3: THUMBNAIL MIGRATION                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌──────────────────┐       ┌──────────────────┐       ┌────────────────┐  │
│   │  thumb/{id}       │       │  Image           │       │  AdHive        │  │
│   │  (JPEG files)     │──────►│  Converter (Go)  │──────►│  Thumbnails    │  │
│   └──────────────────┘       └──────────────────┘       └────────────────┘  │
│                                                                              │
│   Process:                                                                   │
│   1. Read JPEG from thumb/{shiori_id}                                       │
│   2. Convert to WebP (AdHive's preferred format)                            │
│   3. Store at data/thumbnails/{entry-uuid}/{new-uuid}.webp                 │
│   4. Create ThumbnailCandidate record                                        │
│   5. Mark candidate as selected                                             │
│   6. Update catalog_entries.thumbnail_path                                  │
│                                                                              │
│   Alternative: Use BoltDB "image" bucket if available                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### New Files to Create

```
adhive/
├── cmd/
│   └── import-shiori/
│       └── main.go              # Import CLI entrypoint
├── internal/
│   └── import/
│       ├── parser.go            # MariaDB dump SQL parser
│       ├── mapper.go            # Shiori → AdHive ID mapping
│       ├── bolt_extractor.go    # BoltDB archive extraction
│       ├── thumbnail.go         # Thumbnail conversion
│       └── import.go            # Main import orchestrator
└── docs/
    └── adr/
        └── 001-shiori-import-architecture.md
```

### CLI Commands

```bash
# Import SQL data only (Phase 1)
./adhive import-shiori --sql=/path/to/dump.sql --dry-run

# Import everything (Phases 1-3)
./adhive import-shiori \
  --sql=/path/to/dump.sql \
  --archives=/path/to/archive/ \
  --thumbs=/path/to/thumb/ \
  --user-id=<target-user-uuid>

# Resume partial import
./adhive import-shiori --resume --mapping=/path/to/id-mapping.json
```

### Key Implementation Details

#### 1. SQL Parser (internal/import/parser.go)

```go
type ShioriBookmark struct {
    ID          int
    URL         string
    Title       string
    Excerpt     string
    Author      string
    Content     string  // Readability text
    HTML        string  // Full archived HTML
    CreatedAt   time.Time
    ModifiedAt  time.Time
    HasContent  bool
    Tags        []string
}

type ShioriTag struct {
    ID   int
    Name string
}

func ParseDumpSQL(filePath string) ([]ShioriBookmark, []ShioriTag, error) {
    // Regex-based parsing of MariaDB INSERT statements
    // Handles escaped characters, multi-line HTML content
}
```

#### 2. BoltDB Extractor (internal/import/bolt_extractor.go)

```go
import "go.etcd.io/bbolt"

type BoltArchive struct {
    HTML   []byte  // From "archive" bucket
    Image  []byte  // From "image" bucket (optional)
}

func ExtractBoltArchive(filePath string) (*BoltArchive, error) {
    db, err := bbolt.Open(filePath, 0600, &bbolt.Options{ReadOnly: true})
    if err != nil {
        return nil, err
    }
    defer db.Close()
    
    var archive BoltArchive
    
    db.View(func(tx *bbolt.Tx) error {
        // Read archive bucket
        if b := tx.Bucket([]byte("archive")); b != nil {
            archive.HTML = b.Get([]byte("data"))
        }
        // Read image bucket (if present)
        if b := tx.Bucket([]byte("image")); b != nil {
            archive.Image = b.Get([]byte("data"))
        }
        return nil
    })
    
    return &archive, nil
}
```

#### 3. ID Mapping (internal/import/mapper.go)

```go
type IDMapper struct {
    shioriToAdHive map[int]string  // Shiori ID → AdHive UUID
    adHiveToShiori map[string]int  // AdHive UUID → Shiori ID (reverse)
    filePath       string          // Path to JSON backup
}

func NewIDMapper() *IDMapper {
    return &IDMapper{
        shioriToAdHive: make(map[int]string),
        adHiveToShiori: make(map[string]int),
    }
}

func (m *IDMapper) Map(shioriID int) string {
    if uuid, ok := m.shioriToAdHive[shioriID]; ok {
        return uuid
    }
    uuid := uuid.New().String()
    m.shioriToAdHive[shioriID] = uuid
    m.adHiveToShiori[uuid] = shioriID
    return uuid
}

func (m *IDMapper) Save() error {
    // Persist mapping to JSON file for resume capability
}

func (m *IDMapper) Load() error {
    // Load existing mapping for resume
}
```

---

## Alternative Approaches Considered

### Alternative 1: Direct MySQL Connection (Rejected)

**Approach:** Connect directly to running Shiori MySQL/MariaDB instance.

**Pros:**
- Simpler parsing (use SQL driver)
- Real-time data access

**Cons:**
- Requires running Shiori instance
- Network dependency
- More complex setup
- Doesn't handle BoltDB archives

**Reason for Rejection:** Everton only has the dump file, not a running Shiori instance.

### Alternative 2: Two-Tool Approach (Rejected)

**Approach:** Use external tools for BoltDB extraction (e.g., Python script).

**Pros:**
- Could leverage existing BoltDB tools
- Language flexibility

**Cons:**
- Tool chain complexity
- Cross-language data passing
- Harder to maintain

**Reason for Rejection:** Keep everything in Go for consistency with AdHive codebase.

### Alternative 3: Incremental Sync (Deferred)

**Approach:** Build a sync mechanism that watches Shiori for changes.

**Pros:**
- Ongoing sync capability
- Live migration

**Cons:**
- Much more complex
- Requires running Shiori instance
- Ongoing maintenance burden

**Reason for Deferral:** One-time migration is sufficient. Sync can be built later if needed.

---

## Trade-offs

| Aspect | Decision | Trade-off |
|--------|----------|-----------|
| **Parser complexity** | Regex-based SQL parser | Simpler, but may need tuning for edge cases in HTML content |
| **Archive extraction** | Go-native BoltDB library | Requires dependency, but stays in ecosystem |
| **Thumbnail format** | Convert JPEG → WebP | Lossy conversion, but consistent with AdHive |
| **ID mapping** | In-memory + JSON backup | Fast, but needs careful error handling |
| **User association** | Single target user | Simpler, but no multi-user support |
| **Archive structure** | AdHive's revision system | Preserves history, but may be over-engineered |

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **BoltDB format changes** | Low | High | Test with actual Shiori v0.8.5 files |
| **HTML parsing failures** | Medium | Medium | Fallback: store raw HTML without asset extraction |
| **Thumbnail conversion issues** | Low | Low | Preserve original JPEGs as candidates |
| **ID collision** | Very Low | Critical | Use UUID v4 (collision resistance) |
| **Missing foreign keys** | Medium | High | Validate all references before insertion |

---

## Success Criteria

1. **Data Completeness:** All 164 bookmarks imported with correct metadata
2. **Tag Preservation:** All 8 tags migrated and linked correctly
3. **Archive Integrity:** BoltDB archives extracted and viewable in AdHive
4. **Thumbnail Quality:** Thumbnails converted and selectable in AdHive UI
5. **Performance:** Import completes in < 5 minutes for 164 bookmarks
6. **Recovery:** Ability to resume interrupted import

---

## Open Questions

1. **Content extraction:** Should we attempt to extract phone numbers and locations from HTML content during import, or rely on user editing later?
   
2. **Archive fidelity:** Shiori archives may be incomplete (missing assets). Should we mark these as `partial` fidelity?

3. **User selection:** Import all bookmarks or allow filtering by tag/date?

4. **Duplicate handling:** If a bookmark URL already exists in AdHive, should we skip, update, or create duplicate?

---

## Implementation Timeline

| Phase | Task | Estimated Time |
|-------|------|-----------------|
| 1 | SQL parser implementation | 4 hours |
| 2 | ID mapping and entry creation | 2 hours |
| 3 | BoltDB extraction | 4 hours |
| 4 | Thumbnail conversion | 2 hours |
| 5 | Archive structure conversion | 3 hours |
| 6 | CLI and integration | 2 hours |
| 7 | Testing with real data | 2 hours |
| **Total** | | **~19 hours** |

---

## Appendix A: Shiori BoltDB Structure Reference

Based on Shiori source code analysis, the BoltDB structure is:

```
archive/{bookmark_id}
├── Bucket: "archive"
│   └── Key: "data" → gzipped WARC-like content
│                       (or raw HTML for older versions)
└── Bucket: "image" (optional)
    └── Key: "data" → screenshot/thumbnail image
```

The archive content may be:
- **Legacy:** Raw HTML string
- **Modern:** Gzipped WARC format

Need to detect format during extraction.

---

## Appendix B: AdHive Archive Structure

```
data/archives/{entry-uuid}/
├── rev-{timestamp}/
│   ├── index.html          # Main page
│   ├── manifest.json       # Asset manifest
│   └── assets/
│       ├── {hash}.css
│       ├── {hash}.js
│       └── {hash}.jpg
```

The manifest.json contains:
```json
{
  "url": "https://original-url.com/page",
  "captured_at": "2026-03-07T10:30:00Z",
  "engine": "playwright",
  "assets": [
    {"url": "https://cdn.com/style.css", "path": "assets/abc123.css", "bytes": 12345}
  ]
}
```

---

*End of ADR-001*