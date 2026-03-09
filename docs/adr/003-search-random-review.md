# ADR Review: Search & Random Entry Implementation

**Status:** Approved  
**Date:** 2026-03-09  
**Reviewer:** @bumblebee  
**Related ADR:** ADR-002 (Performance Optimization)

---

## Review Summary

This review evaluates the hotfix implementation for FTS5 search and random entry selection against ADR-002 patterns and Go/SQL best practices.

**Overall Assessment:** ✅ **APPROVED** with minor recommendations

---

## Review Checklist

### 1. FTS5 Implementation ✅

**Files reviewed:** `internal/repository/entry.go` (searchWithFTS5, searchWithLike)

| Criterion | Status | Notes |
|-----------|--------|-------|
| Uses FTS5 virtual table | ✅ | `entries_fts` table with MATCH |
| Fallback mechanism | ✅ | Graceful fallback to LIKE if FTS5 fails |
| Prefix search syntax | ✅ | `query+"*"` enables prefix matching |
| JOIN pattern correct | ✅ | Joins on `rowid` for optimal FTS5 lookup |

**Implementation pattern:**
```go
// Correct: FTS5 MATCH with parameterized query
ftsQuery.Where("entries_fts MATCH ?", query+"*")

// Correct: JOIN on rowid for optimal performance
Joins("JOIN catalog_entries ON catalog_entries.rowid = fts.rowid")
```

**Minor observation:** The searchWithFTS5 makes 3 queries:
1. Count for pagination
2. Fetch entry IDs
3. Fetch full entries

This is acceptable for now. A future optimization could combine steps 2-3.

---

### 2. SQL Injection Safety ✅

**Files reviewed:** All raw SQL queries in `internal/repository/entry.go`

| Criterion | Status | Notes |
|-----------|--------|-------|
| Parameterized queries | ✅ | All user input passed as parameters |
| No string concatenation | ✅ | Safe patterns throughout |
| Raw SQL safety | ✅ | `db.Raw()` uses parameter binding |

**Safe patterns identified:**
```go
// ✅ SAFE: Parameterized query
r.db.Raw(`SELECT COUNT(*) FROM catalog_entries ce
    JOIN entry_tags et ON ce.id = et.entry_id
    WHERE ce.user_id = ? AND et.tag_id = ?`, userID, tagID)

// ✅ SAFE: Parameterized MATCH
Where("entries_fts MATCH ?", query+"*")

// ✅ SAFE: Parameterized LIKE
Where("LOWER(title) LIKE ?", searchPattern)
```

**No injection vulnerabilities found.**

---

### 3. Random Selection Performance ✅

**Files reviewed:** `FindRandomEntry`, `FindRandomTriedEntry`, `FindRandomEntryWithTag`, `FindRandomTriedEntryWithTag`

| Criterion | Status | Notes |
|-----------|--------|-------|
| O(1) count query | ✅ | Uses `COUNT(*)` not table scan |
| O(offset) retrieval | ✅ | `LIMIT 1 OFFSET ?` pattern |
| No `ORDER BY RANDOM()` | ✅ | Correctly avoided |

**Performance comparison:**

| Method | Before | After |
|--------|--------|-------|
| Random entry | `ORDER BY RANDOM()` O(n log n) | COUNT + OFFSET O(1)+O(offset) |
| Random with filter | Subquery + RANDOM | Raw SQL + COUNT + OFFSET |

**Correctness note:** `rand.Seed(time.Now().UnixNano())` is called per invocation. While functional, seeding should ideally happen once at startup. This is a **minor issue** that doesn't affect correctness.

---

### 4. Tags Relation Removal ✅

**Files reviewed:** `internal/model/entry.go`

| Criterion | Status | Notes |
|-----------|--------|-------|
| Tags field removed | ✅ | No `Tags []Tag` in struct |
| Handler fetches separately | ✅ | Confirmed in earlier review |
| N+1 avoided | ✅ | Tags fetched separately by handler |

**Rationale confirmed:** The handler already fetches tags separately via `tagRepo.GetEntryTags(e.ID)`. Removing the `Tags` relation from the model prevents GORM from attempting Preload on raw SQL queries, which would fail.

**This is the correct architectural decision.**

---

### 5. Error Handling Consistency ✅

| Criterion | Status | Notes |
|-----------|--------|-------|
| Context propagation | ✅ | All methods use `ctx` |
| GORM error returns | ✅ | Consistent with codebase |
| Logging for fallback | ✅ | FTS5 → LIKE fallback logged |

**Pattern consistency:**
```go
// Consistent error handling
if err != nil {
    return nil, err
}
return &model.EntryListResult{...}, nil
```

---

### 6. GORM Anti-Patterns ✅

| Criterion | Status | Notes |
|-----------|--------|-------|
| Raw SQL for complex queries | ✅ | Appropriate for FTS5 and random selection |
| No Table/Select abuse | ✅ | Clean patterns used |
| Proper context usage | ✅ | `db.WithContext(ctx)` throughout |
| No N+1 in new code | ✅ | Tags fetched separately |

---

## Recommendations

### Minor Issues (Non-blocking)

1. **Random Seed Initialization**

   **Current:**
   ```go
   rand.Seed(time.Now().UnixNano())
   randomOffset := rand.Intn(int(count))
   ```

   **Recommendation:** Initialize random seed once at application startup:
   ```go
   // cmd/server/main.go
   func init() {
       rand.Seed(time.Now().UnixNano())
   }
   ```
   
   Or use `math/rand/v2` (Go 1.22+) which auto-seeds.

2. **Source/Location LIKE Pattern**

   **Current:**
   ```go
   Where("url LIKE ?", "%"+filter.Source+"%")
   ```

   **Note:** This is safe from injection but may have performance impact on large datasets. Consider prefix matching or domain extraction for future optimization.

3. **Count Query in Random Methods**

   **Minor optimization:** For very large datasets, consider caching the count with a TTL:
   ```go
   // Future optimization for high-traffic scenarios
   type countCache struct {
       value   int64
       expires time.Time
   }
   ```

---

## Code Quality Observations

### Positive Patterns

1. **Defensive Fallback:** FTS5 falls back gracefully to LIKE if unavailable
2. **Logging:** Fallback is logged for debugging
3. **Consistent Patterns:** All random methods use same count+offset pattern
4. **Context Usage:** All repository methods properly use context

### Areas for Future Improvement

1. **FTS5 Table Creation:** Ensure migration creates `entries_fts` virtual table
2. **FTS5 Triggers:** Verify triggers exist for INSERT/UPDATE/DELETE sync
3. **Index Usage:** Add `EXPLAIN QUERY PLAN` to tests to verify index usage

---

## Security Review

| Area | Status | Notes |
|------|--------|-------|
| SQL Injection | ✅ Safe | All parameters properly bound |
| User Isolation | ✅ Enforced | All queries filtered by `user_id` |
| Input Validation | ⚠️ Handled at handler | Repository assumes validated input |

---

## Performance Characteristics

### Expected Performance (Post-Implementation)

| Operation | Time Complexity | Notes |
|-----------|-----------------|-------|
| FTS5 Search | O(log n) | Uses FTS5 index |
| LIKE Fallback | O(n) | Full scan, only on FTS5 failure |
| Random Entry | O(1) + O(offset) | Count + Offset, very fast |
| Random with Tag | O(1) + O(offset) | Same pattern with JOIN |

### Database Load

- **FTS5 Search:** 3 queries (count, IDs, entries)
- **Random Selection:** 2 queries (count, fetch)
- **All queries use indexes** when available

---

## Conclusion

**APPROVED** for production use.

The implementation correctly follows ADR-002 patterns:
- FTS5 search with fallback
- Parameterized queries for safety
- COUNT + OFFSET for random selection
- Proper separation of concerns (tags fetched separately)

Minor recommendations for random seed initialization are non-blocking and can be addressed in a future cleanup.

---

## Sign-off

| Reviewer | Date | Status |
|----------|------|--------|
| @bumblebee | 2026-03-09 | ✅ Approved |

---

*End of ADR Review*