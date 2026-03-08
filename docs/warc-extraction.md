# WARC/BoltDB Extraction Investigation - Technical Report

> **Task:** Investigate extraction methods for Shiori's archive format  
> **Status:** ✅ Complete  
> **PoC:** Verified working extraction

---

## Summary

Shiori stores archived web pages in **BoltDB files** (not standard WARC). Each archive file contains multiple buckets with gzip-compressed content. We successfully extracted HTML content using the `go-shiori/warc` library.

---

## Archive Format Analysis

### Structure

| Aspect | Details |
|--------|---------|
| **File type** | BoltDB (key-value store) |
| **Location** | `~/Projects/shiori/data/archive/{bookmarkID}` |
| **Total archives** | ~180 files (varying sizes: 128KB - 60MB) |
| **Content encoding** | Gzip compressed |
| **Main bucket** | `archive-root` (contains main HTML) |
| **Asset buckets** | URL-safe keys (e.g., `https-garotacomlocal.com-wp-content-uploads-...`) |

### Bucket Schema

Each bucket contains:
- `content` — gzip-compressed binary data (HTML, images, CSS, JS, etc.)
- `type` — MIME type string (e.g., `text/html; charset=UTF-8`)

---

## Extraction Methods

### Option 1: Go with go-shiori/warc (Recommended)

**Dependencies:**
```go
import (
    "github.com/go-shiori/warc"
    "go.etcd.io/bbolt"
)
```

**Code:**
```go
// Open archive
archive, err := warc.Open("data/archive/10")
if err != nil {
    log.Fatal(err)
}
defer archive.Close()

// Read main HTML content
content, contentType, err := archive.Read("archive-root")
if err != nil {
    log.Fatal(err)
}

// Decompress gzip content
decompressed := decompressGzip(content)

// Save
os.WriteFile("index.html", decompressed, 0644)
```

### Option 2: Direct BoltDB Access

For fine-grained control or batch extraction:

```go
db, _ := bbolt.Open("data/archive/10", 0444, nil)
defer db.Close()

db.View(func(tx *bbolt.Tx) error {
    return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
        content := bucket.Get([]byte("content"))
        contentType := string(bucket.Get([]byte("type")))
        // Process content...
        return nil
    })
})
```

---

## Verified PoC Results

We successfully extracted archive #10:

```
Saved HTML: 72,805 bytes (decompressed from 13,956 bytes gzip)
```

Sample extracted content:
```html
<!DOCTYPE html><html lang="pt-BR"><head>
  <meta charset="UTF-8"/>
  <title>Yumi Oshiro ⋆ Garota Com Local</title>
  ...
</head>
<body>...
```

---

## Asset Handling

To extract all assets (images, CSS, JS):

1. Iterate all buckets using BoltDB directly
2. Map bucket names back to original URLs
3. Decompress each bucket's content
4. Write to filesystem with appropriate extensions

The bucket naming convention:
- `https://example.com/page` → bucket `https-example.com-page`
- Special chars encoded as-is (dashes)

---

## Migration to AdHive

For importing Shiori archives into AdHive:

1. **Read bookmarks** from Shiori's database (MySQL/PostgreSQL/SQLite)
2. **Extract URLs** and map to archive file IDs
3. **Use warc library** to read HTML content
4. **Parse and sanitize** content (remove relative URLs, fix asset paths)
5. **Import** into AdHive's `catalog_entries` table with:
   - `url` — original URL
   - `title` — from Shiori bookmark
   - `html_content` — extracted HTML
   - `archive_status` = "completed"

---

## Alternative: Python Script

For simpler integration or migration scripts:

```python
# Requires: warcio, boltdb
from warcio.archiveiterator import ArchiveIterator
import boltdb

# Direct BoltDB read
db = boltdb.BoltDB('archive/10')
content = db.get('archive-root', 'content')
# Decompress gzip...
```

---

## Recommendations

| Priority | Action |
|----------|--------|
| **High** | Use Go + warc library for production extraction |
| **Medium** | Build a migration CLI that: reads Shiori DB, extracts content, imports to AdHive |
| **Low** | Consider extracting assets to serve from AdHive's file storage |

---

## Files Referenced

- Archive files: `~/Projects/shiori/data/archive/*`
- WARC library: `github.com/go-shiori/warc`
- BoltDB: `go.etcd.io/bbolt`
- Shiori archiver: `internal/domains/archiver.go`

---

*Delivered by Megatron (SE Team)*
