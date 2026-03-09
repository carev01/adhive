# ADR-002: Performance & Query Optimization Plan

**Status:** Proposed  
**Date:** 2026-03-08  
**Decision Makers:** @bumblebee, @jarvis  
**Supersedes:** N/A

---

## Executive Summary

This ADR identifies performance bottlenecks in AdHive and proposes a prioritized optimization plan. The analysis covers database queries, API efficiency, caching strategies, and frontend performance.

**Critical Findings:**
- **N+1 Query Problem**: Entry list endpoint makes 1 + N queries for tags
- **Missing Indexes**: No composite indexes for common filter/sort patterns
- **Suboptimal Search**: LIKE queries cannot use indexes
- **No SQLite Tuning**: Default configuration without WAL mode or cache optimization
- **Missing FTS5**: Full-text search not implemented despite docs mentioning it

---

## Table of Contents

1. [Current State Analysis](#current-state-analysis)
2. [Database Optimization](#database-optimization)
3. [Query Optimization](#query-optimization)
4. [API Performance](#api-performance)
5. [Frontend Performance](#frontend-performance)
6. [Caching Strategy](#caching-strategy)
7. [Implementation Plan](#implementation-plan)
8. [Monitoring Recommendations](#monitoring-recommendations)

---

## Current State Analysis

### Database Schema (Current)

```sql
-- catalog_entries (current indexes)
CREATE INDEX idx_catalog_entries_user_id ON catalog_entries(user_id);
CREATE INDEX idx_catalog_entries_archive_current_revision_id ON catalog_entries(archive_current_revision_id);

-- Missing indexes for:
-- • user_id + created_at (ordered pagination)
-- • user_id + archive_status (status filter)
-- • user_id + location (location filter)
-- • Full-text search on title/description
```

### Query Patterns Analysis

| Endpoint | Query Pattern | Issues |
|----------|---------------|--------|
| `GET /entries` | List with filters | N+1 tag queries, subquery filters |
| `GET /entries?search=` | LIKE search | No index usage, full table scan |
| `GET /entries/random` | `ORDER BY RANDOM()` | Full table scan |
| `GET /tags` | With counts | N+1 count queries |
| `GET /entries/:id` | Single fetch | OK - uses primary key |

### Performance Baseline (Estimated)

| Operation | Current (est.) | Target |
|-----------|----------------|--------|
| List 100 entries | ~150ms | <50ms |
| Search (LIKE) | ~200ms | <30ms |
| Random entry | ~50ms | <10ms |
| Tag list with counts | ~30ms | <10ms |

*Note: Actual benchmarks should be run against production data*

---

## Database Optimization

### 1. Enable SQLite WAL Mode

**Problem:** Default rollback journal mode causes write blocking during reads.

**Solution:** Enable Write-Ahead Logging (WAL) mode.

```go
// cmd/server/main.go - add after db connection
func configureSQLite(db *gorm.DB) error {
    sqlDB, err := db.DB()
    if err != nil {
        return err
    }
    
    // Enable WAL mode for better concurrent performance
    _, err = sqlDB.Exec("PRAGMA journal_mode=WAL;")
    if err != nil {
        return err
    }
    
    // Increase cache size (negative = kilobytes)
    _, err = sqlDB.Exec("PRAGMA cache_size = -64000;") // 64MB cache
    if err != nil {
        return err
    }
    
    // Set busy timeout for concurrent access
    _, err = sqlDB.Exec("PRAGMA busy_timeout = 5000;") // 5 seconds
    if err != nil {
        return err
    }
    
    // Enable foreign key enforcement
    _, err = sqlDB.Exec("PRAGMA foreign_keys = ON;")
    if err != nil {
        return err
    }
    
    // Synchronous mode for safety (can use NORMAL for speed)
    _, err = sqlDB.Exec("PRAGMA synchronous = NORMAL;")
    if err != nil {
        return err
    }
    
    return nil
}
```

**Impact:** 30-50% improvement in concurrent read performance.

---

### 2. Add Composite Indexes

**Problem:** Common filter/sort patterns lack proper index coverage.

**Solution:** Create migration for composite indexes.

```go
// migrations/XXXX_add_performance_indexes.go

func Up(db *gorm.DB) error {
    // User's entries ordered by creation date (most common pattern)
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_entries_user_created 
        ON catalog_entries(user_id, created_at DESC);
    `).Error; err != nil {
        return err
    }
    
    // User's entries filtered by status + ordered by date
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_entries_user_status_created 
        ON catalog_entries(user_id, archive_status, created_at DESC);
    `).Error; err != nil {
        return err
    }
    
    // User's entries filtered by location
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_entries_user_location 
        ON catalog_entries(user_id, location);
    `).Error; err != nil {
        return err
    }
    
    // Interaction lookups by user (for exclude_tried filter)
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_interactions_user_tried 
        ON interactions(user_id, tried, entry_id);
    `).Error; err != nil {
        return err
    }
    
    // Interaction lookups by entry + user
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_interactions_entry_user 
        ON interactions(entry_id, user_id);
    `).Error; err != nil {
        return err
    }
    
    // Tag lookups by user with name ordering
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_tags_user_name 
        ON tags(user_id, name);
    `).Error; err != nil {
        return err
    }
    
    // Entry-tag junction table for efficient tag filtering
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_entry_tags_entry_tag 
        ON entry_tags(entry_id, tag_id);
    `).Error; err != nil {
        return err
    }
    
    if err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_entry_tags_tag_entry 
        ON entry_tags(tag_id, entry_id);
    `).Error; err != nil {
        return err
    }
    
    return nil
}
```

**Impact:** 50-80% improvement in filtered list queries.

---

### 3. Implement FTS5 Full-Text Search

**Problem:** LIKE queries with `%term%` cannot use indexes and cause full table scans.

**Solution:** Implement SQLite FTS5 virtual table for full-text search.

```sql
-- Create FTS5 virtual table
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
    title,
    description,
    phone_number,
    location,
    url,
    content=catalog_entries,
    content_rowid=rowid
);

-- Trigger to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON catalog_entries
BEGIN
    INSERT INTO entries_fts(rowid, title, description, phone_number, location, url)
    VALUES (new.rowid, new.title, new.description, new.phone_number, new.location, new.url);
END;

CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON catalog_entries
BEGIN
    INSERT INTO entries_fts(entries_fts, rowid, title, description, phone_number, location, url)
    VALUES ('delete', old.rowid, old.title, old.description, old.phone_number, old.location, old.url);
END;

CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE ON catalog_entries
BEGIN
    INSERT INTO entries_fts(entries_fts, rowid, title, description, phone_number, location, url)
    VALUES ('delete', old.rowid, old.title, old.description, old.phone_number, old.location, old.url);
    INSERT INTO entries_fts(rowid, title, description, phone_number, location, url)
    VALUES (new.rowid, new.title, new.description, new.phone_number, new.location, new.url);
END;
```

**Updated Repository Method:**

```go
// repository/entry.go

func (r *EntryRepository) Search(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) {
    // Use FTS5 for search
    baseQuery := r.db.WithContext(ctx).
        Table("catalog_entries ce").
        Joins("JOIN entries_fts fts ON ce.rowid = fts.rowid").
        Where("ce.user_id = ?", userID).
        Where("entries_fts MATCH ?", query+"*") // FTS5 match syntax
    
    // Apply additional filters...
    
    // Count and paginate...
}
```

**Impact:** 10x-100x improvement in search performance.

---

## Query Optimization

### 1. Fix N+1 Query in Entry List

**Problem:** Each entry fetches tags separately.

```go
// Current (handler/entry.go - List method)
for i, e := range result.Entries {
    tags, _ := h.tagRepo.GetEntryTags(e.ID)  // N+1 queries!
}
```

**Solution:** Preload tags with GORM or use a single join query.

```go
// repository/entry.go

func (r *EntryRepository) GetByUserIDWithTags(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
    // First get entry IDs with pagination
    query := r.db.WithContext(ctx).Where("user_id = ?", userID)
    
    // Apply filters (same as before)...
    
    // Get total count
    var total int64
    query.Model(&model.CatalogEntry{}).Count(&total)
    
    // Get entries with tags in single query
    offset := (filter.Page - 1) * filter.Limit
    
    type EntryWithTags struct {
        model.CatalogEntry
        TagsJSON string // Will be populated by join
    }
    
    var entries []EntryWithTags
    err := r.db.WithContext(ctx).
        Table("catalog_entries ce").
        Select(`ce.*, GROUP_CONCAT(t.id || ':' || t.name || ':' || COALESCE(t.color, '')) as tags_json`).
        Joins("LEFT JOIN entry_tags et ON ce.id = et.entry_id").
        Joins("LEFT JOIN tags t ON et.tag_id = t.id").
        Where("ce.user_id = ?", userID).
        // Apply filters...
        Group("ce.id").
        Order(sortField + " " + sortDir).
        Offset(offset).
        Limit(filter.Limit).
        Find(&entries).Error
    
    // Parse results and return...
}
```

**Alternative (simpler):** Use GORM's Preload:

```go
func (r *EntryRepository) GetByUserID(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
    // ...filter logic...
    
    var entries []*model.CatalogEntry
    err := query.
        Preload("Tags"). // Requires adding Tags field to CatalogEntry model
        Order(sortField + " " + sortDir).
        Offset(offset).
        Limit(filter.Limit).
        Find(&entries).Error
    
    // ...
}
```

**Impact:** Reduces N+1 queries to 2 queries. For 100 entries: 101 queries → 2 queries.

---

### 2. Optimize Tag Count Query

**Problem:** N queries for tag counts.

```go
// Current (repository/tag.go)
func (r *TagRepository) GetTagsWithCount(userID string) ([]model.TagWithCount, error) {
    // ... fetch tags ...
    for i, tag := range tags {
        var count int64
        r.db.Model(&model.EntryTag{}).Where("tag_id = ?", tag.ID).Count(&count)  // N queries!
        result[i].Count = count
    }
}
```

**Solution:** Single query with GROUP BY.

```go
func (r *TagRepository) GetTagsWithCount(userID string) ([]model.TagWithCount, error) {
    var results []model.TagWithCount
    
    err := r.db.WithContext(ctx).
        Table("tags t").
        Select("t.*, COUNT(et.entry_id) as count").
        Joins("LEFT JOIN entry_tags et ON t.id = et.tag_id").
        Joins("LEFT JOIN catalog_entries ce ON et.entry_id = ce.id AND ce.user_id = ?", userID).
        Where("t.user_id = ?", userID).
        Group("t.id").
        Order("t.name ASC").
        Find(&results).Error
    
    return results, err
}
```

**Impact:** N queries → 1 query. For 50 tags: 50 queries → 1 query.

---

### 3. Optimize Subquery Filters

**Problem:** Correlated subqueries are slow.

```go
// Current (repository/entry.go)
if filter.ExcludeTried {
    query = query.Where("id NOT IN (SELECT entry_id FROM interactions WHERE user_id = ? AND tried = ?)", userID, true)
}
```

**Solution:** Use LEFT JOIN with NULL check.

```go
if filter.ExcludeTried {
    query = query.
        Joins("LEFT JOIN interactions i ON catalog_entries.id = i.entry_id AND i.user_id = ? AND i.tried = ?", userID, true).
        Where("i.id IS NULL")
}
```

**Impact:** 2-5x improvement for filtered queries.

---

### 4. Optimize Random Entry Selection

**Problem:** `ORDER BY RANDOM()` causes full table scan.

```go
// Current
r.db.Where("user_id = ?", userID).Order("RANDOM()").Take(&entry)
```

**Solution:** Use COUNT + OFFSET with random position.

```go
func (r *EntryRepository) FindRandomEntry(ctx context.Context, userID string) (*model.CatalogEntry, error) {
    // Get total count first (can be cached)
    var count int64
    r.db.WithContext(ctx).Model(&model.CatalogEntry{}).
        Where("user_id = ?", userID).
        Count(&count)
    
    if count == 0 {
        return nil, gorm.ErrRecordNotFound
    }
    
    // Get random offset
    randomOffset := rand.Intn(int(count)) // #nosec G404 - not crypto
    
    var entry model.CatalogEntry
    err := r.db.WithContext(ctx).
        Where("user_id = ?", userID).
        Offset(randomOffset).
        First(&entry).Error
    
    return &entry, err
}
```

**Alternative:** Use SQLite's `ABS(RANDOM() % count)` pattern in single query.

**Impact:** O(1) instead of O(n log n) for random selection.

---

## API Performance

### 1. Response Compression

**Problem:** Large entry lists with descriptions can be 100KB+.

**Solution:** Enable gzip compression in Gin.

```go
// middleware/compression.go

import "github.com/gin-contrib/gzip"

func setupRouter(...) *gin.Engine {
    r := gin.Default()
    
    // Enable gzip compression for responses > 1KB
    r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{
        "/files/", // Binary files already compressed
    })))
    
    // ...
}
```

**Impact:** 70-90% reduction in response size for text/json.

---

### 2. Pagination Improvements

**Problem:** Count queries are slow on large datasets.

**Solution:** Use cursor-based pagination for infinite scroll, or estimate counts.

```go
// Estimate count instead of exact
func (r *EntryRepository) GetByUserID(ctx context.Context, userID string, filter *model.EntryFilter) (*model.EntryListResult, error) {
    // ...build query...
    
    // For large datasets, use approximate count
    var total int64
    if filter.Page == 1 {
        // First page: exact count
        query.Model(&model.CatalogEntry{}).Count(&total)
    } else {
        // Subsequent pages: estimate from index stats
        // SQLite doesn't have easy estimate, so we keep exact count
        query.Model(&model.CatalogEntry{}).Count(&total)
    }
    
    // ...
}
```

**Alternative for large scale:** Use `total > (page * limit)` boolean instead of exact count.

---

### 3. Field Selection (Sparse Fieldsets)

**Problem:** List endpoint returns full descriptions which may not be needed.

**Solution:** Add sparse fieldsets support.

```go
type EntryListParams struct {
    // ... existing params ...
    Fields string `form:"fields"` // e.g., "id,title,url"
}

func (r *EntryRepository) GetByUserID(ctx context.Context, userID string, filter *model.EntryFilter, fields []string) (*model.EntryListResult, error) {
    query := r.db.WithContext(ctx).Where("user_id = ?", userID)
    
    // Select only requested fields
    if len(fields) > 0 {
        query = query.Select(fields)
    }
    
    // ...
}
```

**Impact:** Reduces payload size by 50-80% for list views.

---

## Frontend Performance

### 1. Bundle Analysis

```bash
# Analyze bundle size
cd frontend
npx vite-bundle-visualizer
```

**Current estimated bundle size:** ~200-300KB (uncompressed)

**Recommendations:**
- Enable code splitting for routes
- Lazy load heavy components (e.g., image viewer)
- Use dynamic imports for modals

### 2. Reduce Re-Renders

**Problem:** Polling triggers full re-render every 30 seconds.

**Solution:** Use Svelte's keyed each with optimistic updates.

```svelte
<!-- Current -->
{#each entries as entry}
    <EntryCard {entry} />
{/each}

<!-- Optimized with key -->
{#each entries as entry (entry.id)}
    <EntryCard {entry} />
{/each}
```

### 3. Debounced Search

**Problem:** Search triggers query on every keystroke.

**Solution:** Already implemented with timeout - verify it's working.

```typescript
// Check this exists in +page.svelte
let searchTimeout: ReturnType<typeof setTimeout>;
$: {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => loadEntries(), 300);
}
```

### 4. Image Lazy Loading

**Solution:** Add loading="lazy" to thumbnail images.

```svelte
<img src={entry.thumbnail_path} alt={entry.title} loading="lazy" decoding="async" />
```

---

## Caching Strategy

### 1. In-Memory Cache (Recommended for Single-User)

**When to use:** Single-user deployments with limited RAM.

```go
// service/cache.go

import "github.com/patrickmn/go-cache"

var entryCache = cache.New(5*time.Minute, 10*time.Minute)

func (r *EntryRepository) GetByID(ctx context.Context, id string) (*model.CatalogEntry, error) {
    cacheKey := "entry:" + id
    if cached, found := entryCache.Get(cacheKey); found {
        return cached.(*model.CatalogEntry), nil
    }
    
    var entry model.CatalogEntry
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
    if err != nil {
        return nil, err
    }
    
    entryCache.Set(cacheKey, &entry, cache.DefaultExpiration)
    return &entry, nil
}
```

### 2. Cache Invalidation

**Strategy:** Write-through cache with TTL.

```go
func (r *EntryRepository) Update(ctx context.Context, entry *model.CatalogEntry) error {
    err := r.db.WithContext(ctx).Save(entry).Error
    if err != nil {
        return err
    }
    
    // Invalidate cache
    entryCache.Delete("entry:" + entry.ID)
    entryCache.Delete("entries:" + entry.UserID)
    
    return nil
}
```

### 3. What to Cache

| Data | TTL | Invalidation |
|------|-----|--------------|
| Single entry | 5 min | On update/delete |
| Tag list | 10 min | On create/delete |
| User's sources | 10 min | On entry create/delete |
| User's locations | 10 min | On entry update |

### 4. What NOT to Cache

- Search results (dynamic)
- Filtered lists (many variations)
- Archive status (changes during archival)

---

## Implementation Plan

### Phase 1: Quick Wins (1-2 days)

| Task | Priority | Effort | Impact |
|------|----------|--------|--------|
| Enable SQLite WAL mode | High | 1h | High |
| Add composite indexes | High | 2h | High |
| Fix N+1 tag queries | High | 2h | High |
| Fix N+1 count queries | High | 1h | Medium |
| Enable gzip compression | Medium | 30m | Medium |

### Phase 2: Search Optimization (2-3 days)

| Task | Priority | Effort | Impact |
|------|----------|--------|--------|
| Implement FTS5 table | High | 4h | Very High |
| Create sync triggers | High | 1h | High |
| Update repository methods | High | 4h | High |
| Add FTS to migrations | High | 2h | High |

### Phase 3: Query Optimization (2-3 days)

| Task | Priority | Effort | Impact |
|------|----------|--------|--------|
| Convert subqueries to JOINs | Medium | 3h | Medium |
| Optimize random selection | Medium | 2h | Medium |
| Add sparse fieldsets | Low | 4h | Medium |
| Implement query result caching | Medium | 4h | Medium |

### Phase 4: Frontend (1-2 days)

| Task | Priority | Effort | Impact |
|------|----------|--------|--------|
| Add lazy loading to images | Low | 1h | Low |
| Implement route code splitting | Low | 2h | Medium |
| Add keyed each blocks | Low | 1h | Low |
| Bundle analysis | Low | 2h | Varies |

---

## Monitoring Recommendations

### 1. Query Logging

```go
// Add slow query logging
func setupDB() *gorm.DB {
    db, _ := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
        Logger: logger.New(
            log.New(os.Stdout, "\r\n", log.LstdFlags),
            logger.Config{
                SlowThreshold: 100 * time.Millisecond, // Log queries > 100ms
                LogLevel:      logger.Warn,
            },
        ),
    })
    return db
}
```

### 2. Add Metrics Endpoint

```go
// GET /api/v1/metrics
type MetricsResponse struct {
    DatabaseSize    int64 `json:"database_size_bytes"`
    EntryCount      int64 `json:"entry_count"`
    ArchiveSize     int64 `json:"archive_size_bytes"`
    CacheHitRate    float64 `json:"cache_hit_rate"`
    AvgQueryTime    float64 `json:"avg_query_time_ms"`
}
```

### 3. Performance Benchmarks

Create a test suite for regression testing:

```go
// internal/benchmark/queries_test.go

func BenchmarkListEntries(b *testing.B) {
    // Setup test data with 1000 entries
    for i := 0; i < b.N; i++ {
        repo.GetByUserID(ctx, userID, &model.EntryFilter{Page: 1, Limit: 20})
    }
}

func BenchmarkSearch(b *testing.B) {
    for i := 0; i < b.N; i++ {
        repo.Search(ctx, userID, "test query", &model.EntryFilter{Page: 1, Limit: 20})
    }
}
```

---

## Appendix A: Index Creation SQL

```sql
-- Run these in order

-- 1. User + created_at (ordered pagination)
CREATE INDEX IF NOT EXISTS idx_entries_user_created 
ON catalog_entries(user_id, created_at DESC);

-- 2. User + status (status filtering)
CREATE INDEX IF NOT EXISTS idx_entries_user_status_created 
ON catalog_entries(user_id, archive_status, created_at DESC);

-- 3. User + location (location filtering)
CREATE INDEX IF NOT EXISTS idx_entries_user_location 
ON catalog_entries(user_id, location);

-- 4. Interactions (exclude_tried filter)
CREATE INDEX IF NOT EXISTS idx_interactions_user_tried 
ON interactions(user_id, tried, entry_id);

-- 5. Interactions (score filter)
CREATE INDEX IF NOT EXISTS idx_interactions_user_score 
ON interactions(user_id, score);

-- 6. Tags (ordered list)
CREATE INDEX IF NOT EXISTS idx_tags_user_name 
ON tags(user_id, name);

-- 7. Entry-tag junction (both directions)
CREATE INDEX IF NOT EXISTS idx_entry_tags_entry_tag 
ON entry_tags(entry_id, tag_id);

CREATE INDEX IF NOT EXISTS idx_entry_tags_tag_entry 
ON entry_tags(tag_id, entry_id);
```

---

## Appendix B: SQLite Configuration Reference

```go
// Recommended SQLite configuration for AdHive

var sqliteConfig = []string{
    "PRAGMA journal_mode=WAL;",           // Better concurrency
    "PRAGMA synchronous=NORMAL;",         // Faster writes, safe enough
    "PRAGMA cache_size=-64000;",          // 64MB cache
    "PRAGMA busy_timeout=5000;",           // 5s wait for locks
    "PRAGMA foreign_keys=ON;",            // Enforce relationships
    "PRAGMA temp_store=MEMORY;",          // In-memory temp tables
    "PRAGMA mmap_size=268435456;",        // 256MB memory map
}
```

---

## Appendix C: Before/After Query Examples

### Entry List Query

**Before (N+1):**
```sql
-- Query 1: Get entries
SELECT * FROM catalog_entries WHERE user_id = ? ORDER BY created_at DESC LIMIT 20;

-- Query 2-21: Get tags for each entry
SELECT t.* FROM tags t 
JOIN entry_tags et ON t.id = et.tag_id 
WHERE et.entry_id = ?; -- Repeated 20 times
```

**After (Single query):**
```sql
SELECT ce.*, GROUP_CONCAT(t.id || ':' || t.name) as tags
FROM catalog_entries ce
LEFT JOIN entry_tags et ON ce.id = et.entry_id
LEFT JOIN tags t ON et.tag_id = t.id
WHERE ce.user_id = ?
GROUP BY ce.id
ORDER BY ce.created_at DESC
LIMIT 20;
```

### Search Query

**Before (LIKE):**
```sql
SELECT * FROM catalog_entries 
WHERE user_id = ? 
AND (LOWER(title) LIKE '%query%' 
     OR LOWER(description) LIKE '%query%')
ORDER BY created_at DESC;
-- Full table scan, no index usage
```

**After (FTS5):**
```sql
SELECT ce.* FROM catalog_entries ce
JOIN entries_fts fts ON ce.rowid = fts.rowid
WHERE ce.user_id = ?
AND entries_fts MATCH 'query*'
ORDER BY ce.created_at DESC;
-- Uses FTS index, 100x faster for large datasets
```

---

*End of ADR-002*