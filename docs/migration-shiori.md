# Migration Tool Specification: Shiori to AdHive

**Task:** Design Import Tooling and Migration Path  
**Status:** Draft  
**Author:** @jazz (DevOps)  
**Target:** AdHive Documentation Project  
**Date:** 2026-03-07

---

## Executive Summary

This document specifies the migration tooling for importing Shiori data (bookmarks, tags, archives, thumbnails) into AdHive. It builds upon [ADR-001: Shiori-to-AdHive Import Architecture](../adr/001-shiori-import-architecture.md).

---

## 1. Import Tool Options

### Option A: CLI Command in AdHive (Recommended)

```
adhive import-shiori [flags]
```

**Pros:**
- Single binary distribution
- Shares Go dependencies with main application
- Easy to version and maintain
- Can leverage existing AdHive config handling

**Cons:**
- Increases binary size
- Must rebuild when import logic changes

### Option B: Standalone Migration Script

```
./scripts/import-shiori.sh --sql dump.sql --archives ./archive
```

**Pros:**
- Independent from main application
- Can run on different schedule
- Simpler for one-off migrations

**Cons:**
- Duplicate Go runtime
- Harder to maintain
- Less integration with AdHive

### Option C: Database Migration + File Copy

Manual SQL insert + shell scripts for files.

**Pros:**
- Maximum control
- No coding required

**Cons:**
- Error-prone
- No rollback capability
- No progress tracking
- Not reproducible

---

## 2. Recommended Approach: Option A

We will implement a CLI command integrated into AdHive's `cmd/` directory:

```
adhive/
├── cmd/
│   ├── server/           # Existing
│   ├── migrate/          # Existing
│   └── import-shiori/   # NEW
│       └── main.go
├── internal/
│   └── import/           # NEW
│       ├── parser.go
│       ├── mapper.go
│       ├── bolt.go
│       └── thumbnail.go
```

---

## 3. CLI Interface Specification

### Command Structure

```bash
# Full import (all phases)
adhive import-shiori \
  --sql=/path/to/shiori-dump.sql \
  --archives=/path/to/shiori-archive/ \
  --thumbs=/path/to/shiori-thumb/ \
  --user-id=<target-user-uuid>

# Phase 1 only (SQL data)
adhive import-shiori --sql=/path/to/dump.sql --user-id=<uuid>

# Resume from checkpoint
adhive import-shiori --resume --checkpoint=/path/to/checkpoint.json

# Dry run (Phase 1)
adhive import-shiori --sql=/path/to/dump.sql --dry-run
```

### Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--sql` | Yes* | — | Path to MariaDB dump file (`.sql`) |
| `--archives` | No | — | Path to Shiori `archive/` directory |
| `--thumbs` | No | — | Path to Shiori `thumb/` directory |
| `--user-id` | Yes | — | Target AdHive user UUID |
| `--resume` | No | false | Resume from checkpoint |
| `--checkpoint` | No | `./import-checkpoint.json` | Checkpoint file path |
| `--dry-run` | No | false | Parse SQL only, don't write to DB |
| `--verbose` | No | false | Detailed logging |
| `--batch-size` | No | 50 | Entries per DB transaction |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Database error |
| 4 | File system error |
| 5 | Partial failure (some items skipped) |

---

## 4. Phase 1: SQL Data Import

### 4.1 Parsing Strategy

Since Everton has a MariaDB dump file (not a live database), we parse the SQL directly.

**Parser Requirements:**
- Handle `INSERT INTO` statements with multi-line HTML content
- Handle escaped characters (`\'`, `\\`, etc.)
- Handle `NULL` values
- Support both `''` and `""` quoted strings

**Shiori Tables to Parse:**

```sql
-- bookmarks
INSERT INTO `bookmark` VALUES (1,'https://example.com/ad','Car for sale','Nice car','owner','0','<p>Content</p>','<html>...','2024-01-01 10:00:00',1,'2024-01-02 12:00:00');

-- tags
INSERT INTO `tag` VALUES (1,'cars'),(2,'real-estate');

-- bookmark_tag
INSERT INTO `bookmark_tag` VALUES (1,1),(1,2);
```

### 4.2 Data Mapping

| Shiori Field | AdHive Field | Notes |
|--------------|--------------|-------|
| `bookmark.id` | (internal) | Use for ID mapping, generate new UUID |
| `bookmark.url` | `CatalogEntry.URL` | Direct mapping |
| `bookmark.title` | `CatalogEntry.Title` | Direct mapping |
| `bookmark.excerpt` | `CatalogEntry.Description` | Direct mapping |
| `bookmark.created_at` | `CatalogEntry.CreatedAt` | Direct mapping |
| `bookmark.has_content` | `CatalogEntry.ArchiveStatus` | If true: `pending`, else `failed` |
| `tag.name` | `Tag.Name` | Case-insensitive dedupe |
| `bookmark_tag` | `EntryTag` | Junction via ID mapping |

### 4.3 ID Mapping

```go
// internal/import/mapper.go
type IDMapper struct {
    ShioriToAdHive map[int]string  // Shiori bookmark ID → AdHive UUID
    FilePath       string          // Checkpoint file
}

func (m *IDMapper) GetAdHiveID(shioriID int) string {
    if uuid, exists := m.ShioriToAdHive[shioriID]; exists {
        return uuid
    }
    uuid := uuid.New().String()
    m.ShioriToAdHive[shioriID] = uuid
    return uuid
}
```

**Checkpoint File Format:**
```json
{
  "version": 1,
  "shiori_to_adhive": {"1": "abc-123", "2": "def-456"},
  "completed_phases": [1],
  "last_entry_id": 2,
  "timestamp": "2026-03-07T10:30:00Z"
}
```

---

## 5. Phase 2: BoltDB Archive Extraction

### 5.1 Archive Directory Structure

```
shiori-data/
├── archive/
│   ├── 1          # BoltDB file for bookmark ID 1
│   ├── 2
│   └── ...
└── thumb/
    ├── 1.jpg
    ├── 2.jpg
    └── ...
```

### 5.2 BoltDB Extraction

```go
// internal/import/bolt.go

type ExtractedArchive struct {
    HTML       []byte  // From "archive" bucket
    ImageData  []byte  // From "image" bucket (optional)
    BookmarkID int     // From filename
}

func ExtractArchive(filepath string) (*ExtractedArchive, error) {
    db, err := bbolt.Open(filepath, 0600, &bbolt.Options{ReadOnly: true})
    if err != nil {
        return nil, fmt.Errorf("failed to open BoltDB: %w", err)
    }
    defer db.Close()

    var result ExtractedArchive

    err = db.View(func(tx *bbolt.Tx) error {
        // Archive bucket
        if archive := tx.Bucket([]byte("archive")); archive != nil {
            result.HTML = archive.Get([]byte("data"))
        }
        // Image bucket
        if img := tx.Bucket([]byte("image")); img != nil {
            result.ImageData = img.Get([]byte("data"))
        }
        return nil
    })

    return &result, err
}
```

### 5.3 Archive Conversion

1. **Parse HTML** to extract asset URLs
2. **Create AdHive archive directory:**
   ```
   data/archives/{entry-uuid}/
   └── rev-{timestamp}/
       ├── index.html
       ├── manifest.json
       └── assets/
   ```
3. **Write manifest:**
   ```json
   {
     "url": "https://original-url.com",
     "captured_at": "2024-01-01T10:00:00Z",
     "engine": "shiori",
     "fidelity": "partial"
   }
   ```
4. **Create `ArchiveRevision` record in DB**

---

## 6. Phase 3: Thumbnail Migration

### 6.1 Source: Thumb Directory

```
thumb/1.jpg  →  JPEG from Shiori
```

### 6.2 Conversion Pipeline

1. Read JPEG from `thumb/{shiori_id}.jpg`
2. Convert to WebP (lossy, quality 85)
3. Store at `data/thumbnails/{adhive-uuid}/{timestamp}.webp`
4. Create `ThumbnailCandidate` record
5. Mark as selected

```go
// internal/import/thumbnail.go

func ConvertThumbnail(srcPath, dstPath string) error {
    img, err := imaging.DecodeFromFile(srcPath)
    if err != nil {
        return err
    }
    
    // Resize to standard thumbnail size
    img = imaging.Thumbnail(img, 300, 200, imaging.Lanczos)
    
    // Convert to WebP
    return imaging.EncodeToFile(img, dstPath, imaging.WebPQuality(85))
}
```

---

## 7. Error Handling

### 7.1 Skip Strategy

| Error | Action |
|-------|--------|
| Corrupt BoltDB file | Log error, skip, continue |
| Missing thumbnail | Log warning, skip thumbnail phase |
| Invalid HTML | Store raw HTML, mark `fidelity: low` |
| DB constraint violation | Log error, skip entry, continue |
| File permission denied | Log error, skip file, continue |

### 7.2 Logging

```bash
# Progress output
[INFO] Starting import...
[INFO] Parsing SQL dump...
[INFO] Found 164 bookmarks, 8 tags
[INFO] Importing bookmarks: 164/164 [====================] 100%
[INFO] Importing tags: 8/8 [==========================] 100%
[INFO] Phase 1 complete: 164 entries, 8 tags
[INFO] Extracting archives: 1/164 ...
[WARN] Skipped corrupt BoltDB file 42: invalid header
[INFO] Extracting archives: 164/164 [==================] 100%
[INFO] Phase 2 complete: 163 archives extracted (1 skipped)
[INFO] Converting thumbnails: 164/164 [================] 100%
[INFO] Import complete: 164 entries, 163 archives, 164 thumbnails

# Failed entries logged to import-errors.json
[
  {"phase": "bolt", "bookmark_id": 42, "error": "invalid header", "timestamp": "..."}
]
```

### 7.3 Resume Capability

1. Write checkpoint after each phase
2. On resume, load checkpoint and skip completed work
3. Track `last_entry_id` for batching

---

## 8. Implementation Plan

### 8.1 File Structure

```
adhive/
├── cmd/
│   └── import-shiori/
│       └── main.go           # CLI entrypoint
├── internal/
│   └── import/
│       ├── parser.go         # SQL parsing (Phase 1)
│       ├── mapper.go          # ID mapping + checkpointing
│       ├── bolt.go           # BoltDB extraction (Phase 2)
│       ├── archive.go        # Archive conversion
│       ├── thumbnail.go      # Thumbnail conversion (Phase 3)
│       └── import.go         # Orchestrator
└── docs/
    └── migration-shiori.md   # This document
```

### 8.2 Dependencies (go.mod additions)

```go
require (
    github.com/go-shiori/warc     v0.1.0  // WARC parsing (if needed)
    github.com/disintegration/imaging  // Image conversion
    github.com/google/uuid         v1.6.0  // UUID generation
)
```

### 8.3 Development Sequence

| Step | Task | Effort |
|------|------|--------|
| 1 | CLI skeleton + flag parsing | 1 hr |
| 2 | SQL parser for bookmarks | 2 hr |
| 3 | SQL parser for tags | 1 hr |
| 4 | ID mapper + checkpointing | 1 hr |
| 5 | BoltDB archive extraction | 2 hr |
| 6 | Archive → AdHive format | 2 hr |
| 7 | Thumbnail conversion | 1 hr |
| 8 | Integration testing | 2 hr |
| **Total** | | **~12 hr** |

---

## 9. User Guide (For Deliverable)

### 9.1 Prerequisites

1. Export Shiori data:
   - Database dump: `mysqldump -u shiori -p shiori > dump.sql`
   - Archive directory: `cp -r /path/to/shiori/data/archive ./`
   - Thumb directory: `cp -r /path/to/shiori/data/thumb ./`

2. Get target user ID:
   ```bash
   adhive user list
   # Copy the UUID for the target user
   ```

### 9.2 Running Import

```bash
# Full import
adhive import-shiori \
  --sql=./dump.sql \
  --archives=./archive \
  --thumbs=./thumb \
  --user-id=<USER-UUID>

# Check progress
tail -f import.log
```

### 9.3 Verification

```bash
# Check entry count
adhive entry list | wc -l  # Should be 164 + existing

# Check tags
adhive tag list

# Verify archives exist
ls data/archives/

# Check import errors (if any)
cat import-errors.json
```

---

## 10. Alternative Approaches

### Option B: Standalone Script

If we want a separate binary:

```
scripts/
├── import-shiori.go      # Standalone entrypoint
└── go.mod               # Minimal dependencies
```

**Trade-off:** More maintenance burden, easier to run independently.

### Option C: API-Based Import

```bash
# Upload dump to running AdHive instance
curl -X POST /api/v1/import/shiori \
  -F "sql=@dump.sql" \
  -F "archives=@archive.tar"
```

**Trade-off:** Requires running server, more complex error handling.

---

## 11. Recommendations

1. **Implement Option A** — Integrated CLI command
2. **Start with Phase 1** — SQL import is the highest value
3. **Add checkpointing early** — Enables resume for large imports
4. **Log failures to JSON** — Enables post-mortem analysis
5. **Test with real data** — Everton has the actual dump file

---

## 12. Open Questions (for team decision)

1. **Content extraction:** Should we parse phone/location from HTML during import, or rely on manual editing?
2. **Duplicate URLs:** If URL exists in AdHive, skip or update?
3. **Archive fidelity:** Mark Shiori archives as `partial` since they may be missing assets?
4. **Batch size:** Is 50 entries/transaction appropriate for 164 items?

---

*End of Specification*
