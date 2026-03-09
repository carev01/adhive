# AdHive Architecture Document

**Version:** 1.0.0  
**Last Updated:** 2026-03-09

---

## Overview

AdHive is a self-hosted catalog application for organizing and reviewing archived classified advertisements. It provides a web-based interface for users to catalog, search, and interact with archived ads from various sources.

### Key Features

- 📁 **Catalog Management** - Organize ads with tags, notes, and custom fields
- 🔍 **Full-Text Search** - Fast FTS5-powered search across titles, descriptions, and notes
- 🎲 **Random Review** - Discover forgotten ads with weighted random selection
- 🖼️ **Thumbnail Management** - Automatic thumbnail extraction with candidate selection
- 📦 **Archive Storage** - Store multiple revisions of archived pages
- 🔐 **User Authentication** - Secure session-based authentication

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Client Layer                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │   Web Browser    │    │   Mobile App     │    │   CLI/Scripts    │      │
│  │   (SvelteKit)    │    │   (Future)       │    │   (Future)       │      │
│  └────────┬─────────┘    └────────┬─────────┘    └────────┬─────────┘      │
└───────────┼───────────────────────┼───────────────────────┼─────────────────┘
            │                       │                       │
            └───────────────────────┼───────────────────────┘
                                    │ HTTP/REST
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API Layer                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                         Gin Router                                    │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐    │  │
│  │  │   Auth      │ │   Entry     │ │    Tag      │ │   File      │    │  │
│  │  │  Handler    │ │  Handler    │ │  Handler    │ │  Handler    │    │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘    │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                     │  │
│  │  │Interaction  │ │  Archive    │ │ Thumbnail   │                     │  │
│  │  │  Handler    │ │   Ops       │ │  Handler    │                     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘                     │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                         Middleware                                    │  │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌─────────┐│  │
│  │  │   Auth    │ │   CORS    │ │Rate Limit │ │  CSRF     │ │Security ││  │
│  │  │Middleware │ │Middleware │ │Middleware │ │Middleware ││ Headers ││  │
│  │  └───────────┘ └───────────┘ └───────────┘ └───────────┘ └─────────┘│  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Service Layer                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │   FileService    │  │ThumbnailService  │  │  ArchiveWorker   │          │
│  │                  │  │                  │  │   (Background)   │          │
│  │  - Upload        │  │  - Candidate     │  │                  │          │
│  │  - Serve         │  │    Generation    │  │  - Playwright    │          │
│  │  - Delete        │  │  - Scoring       │  │  - Screenshot    │          │
│  │                  │  │                  │  │  - Asset Extract │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Repository Layer                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │   EntryRepo      │  │    TagRepo       │  │InteractionRepo   │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │   UserRepo       │  │  SessionRepo     │  │ArchiveRevision   │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│  ┌──────────────────┐  ┌──────────────────┐                                │
│  │ ArchiveAssetRepo │  │ThumbnailCandidate│                                │
│  └──────────────────┘  └──────────────────┘                                │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Storage Layer                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────┐  ┌──────────────────────────────┐        │
│  │      SQLite Database         │  │       File System            │        │
│  │                              │  │                              │        │
│  │  ┌────────────────────────┐  │  │  ┌────────────────────────┐  │        │
│  │  │   catalog_entries      │  │  │  │    data/archives/      │  │        │
│  │  │   tags                 │  │  │  │    data/thumbnails/    │  │        │
│  │  │   entry_tags           │  │  │  │    data/temp/          │  │        │
│  │  │   interactions         │  │  │  └────────────────────────┘  │        │
│  │  │   archive_revisions    │  │  │                              │        │
│  │  │   archive_assets       │  │  │                              │        │
│  │  │   thumbnail_candidates │  │  │                              │        │
│  │  │   users                │  │  │                              │        │
│  │  │   sessions             │  │  │                              │        │
│  │  │   entries_fts (FTS5)   │  │  │                              │        │
│  │  └────────────────────────┘  │  │                              │        │
│  └──────────────────────────────┘  └──────────────────────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Data Model

### Entity Relationship Diagram

```
┌─────────────────┐       ┌─────────────────┐
│     users       │       │    sessions     │
├─────────────────┤       ├─────────────────┤
│ id (PK)         │───┐   │ id (PK)         │
│ email           │   │   │ user_id (FK)    │───┐
│ password_hash   │   │   │ expires_at      │   │
│ display_name    │   │   │ created_at      │   │
│ is_active       │   │   └─────────────────┘   │
│ created_at      │   │                         │
│ updated_at      │   │                         │
└─────────────────┘   │                         │
        │             └─────────────────────────┘
        │
        │ 1:N
        ▼
┌─────────────────────┐       ┌─────────────────────┐
│   catalog_entries   │       │        tags         │
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK)             │
│ user_id (FK)        │───┐   │ user_id (FK)        │───┐
│ url                 │   │   │ name                │   │
│ title               │   │   │ color               │   │
│ description         │   │   │ created_at          │   │
│ phone_number        │   │   └─────────────────────┘   │
│ location            │   │             │               │
│ thumbnail_path      │   │             │               │
│ archive_path        │   │             │ M:N           │
│ archive_status      │   │             │               │
│ archive_fidelity    │   │             ▼               │
│ archive_current_    │   │   ┌─────────────────────┐   │
│   revision_id       │   │   │    entry_tags       │   │
│ thumbnail_source    │   │   ├─────────────────────┤   │
│ created_at          │   │   │ entry_id (FK)       │───┘
│ updated_at          │   │   │ tag_id (FK)         │
└─────────────────────┘   │   └─────────────────────┘
        │                 │
        │ 1:N             └─────────────────────────────
        ▼
┌─────────────────────┐
│   interactions      │
├─────────────────────┤
│ id (PK)             │
│ entry_id (FK)       │
│ user_id (FK)        │
│ tried               │
│ score               │
│ comments            │
│ contacted_at        │
│ purchased_at        │
│ created_at          │
│ updated_at          │
└─────────────────────┘
        │
        │ 1:N (via entry)
        ▼
┌─────────────────────┐       ┌─────────────────────┐
│  archive_revisions  │       │   archive_assets    │
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK)             │
│ entry_id (FK)       │       │ revision_id (FK)    │
│ captured_at         │       │ source_url          │
│ root_path           │       │ local_path          │
│ fidelity            │       │ content_type        │
│ notes               │       │ size_bytes          │
│ created_at          │       │ created_at          │
└─────────────────────┘       └─────────────────────┘
        │
        │ 1:N
        ▼
┌─────────────────────┐
│thumbnail_candidates │
├─────────────────────┤
│ id (PK)             │
│ entry_id (FK)       │
│ revision_id (FK)    │
│ source_type         │
│ path                │
│ score               │
│ selected            │
│ created_at          │
└─────────────────────┘
```

### Core Entities

| Entity | Description |
|--------|-------------|
| `users` | User accounts with email/password authentication |
| `sessions` | Session tokens for authenticated users |
| `catalog_entries` | Main ad catalog entries |
| `tags` | User-defined tags for categorization |
| `entry_tags` | Many-to-many relationship between entries and tags |
| `interactions` | User interactions with entries (tried, score, comments) |
| `archive_revisions` | Archive snapshots with timestamps |
| `archive_assets` | Individual files within an archive revision |
| `thumbnail_candidates` | Generated thumbnail candidates with scoring |

---

## Component Details

### 1. API Layer (Handlers)

#### AuthHandler
- User registration and login
- Session management via secure HTTP-only cookies
- CSRF token generation

#### EntryHandler
- CRUD operations for catalog entries
- Full-text search with FTS5
- Random entry selection with filters
- Bulk operations (tag, delete, archive)

#### TagHandler
- CRUD operations for tags
- Tag assignment to entries
- Tag usage counts

#### InteractionHandler
- Track user interactions with entries
- Store tried status, scores, and comments

#### ArchiveOpsHandler
- Archive revision management
- Manual archive refresh triggers
- Archive metrics and monitoring

#### ThumbnailHandler
- Thumbnail candidate generation
- Candidate selection and management

#### FileHandler
- Archive file upload and serving
- Thumbnail upload and serving
- Storage statistics

### 2. Middleware

| Middleware | Purpose |
|------------|---------|
| Auth | Session-based authentication |
| CORS | Cross-origin request handling |
| RateLimit | Request rate limiting (100/min global, 5/min auth) |
| CSRF | Cross-site request forgery protection |
| SecurityHeaders | X-Frame-Options, CSP, etc. |
| InputSanitizer | Request input sanitization |
| RawPathTraversalGuard | Path traversal attack prevention |

### 3. Services

#### FileService
- Manages archive and thumbnail storage
- Handles file upload, retrieval, and deletion
- Generates storage statistics

#### ThumbnailService
- Generates thumbnail candidates from images
- Scores candidates based on quality metrics
- Supports WebP output format

#### ArchiveWorker
- Background worker for page archiving
- Uses Playwright for headless browser capture
- Extracts assets (images, CSS, etc.)

### 4. Repositories

Each repository provides data access for its corresponding entity:

| Repository | Key Methods |
|------------|-------------|
| EntryRepository | Create, GetByID, GetByUserID, Search, FindRandom* |
| TagRepository | Create, FindByUserID, GetEntryTags, GetTagsWithCount |
| InteractionRepository | GetByEntryAndUser, Upsert, Delete |
| ArchiveRevisionRepository | Create, ListByEntryID, Metrics |
| ArchiveAssetRepository | Create, ListByRevisionID, ListImageAssetsByEntry |
| ThumbnailCandidateRepository | Create, ListByEntryID, Select |

---

## Security Architecture

### Authentication

- **Method:** Session-based with HTTP-only cookies
- **Cookie Attributes:** `Secure`, `HttpOnly`, `SameSite=Strict`
- **Session TTL:** 7 days
- **Session Regeneration:** On login/register (prevents fixation)

### Authorization

- User ownership validation on all resources
- Middleware-based authentication check
- CSRF protection on state-changing requests

### Input Validation

- UUID validation on path parameters
- Email format validation
- Password strength requirements (min 8 chars)
- File type validation for uploads
- Path traversal prevention

### Rate Limiting

| Endpoint Type | Limit | Window |
|---------------|-------|--------|
| Global | 100 requests | 1 minute |
| Auth (login) | 5 requests | 1 minute |
| Auth (register) | 3 requests | 1 hour |

### Security Headers

```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'
```

---

## Performance Considerations

### Database Optimization

- **WAL Mode:** Write-Ahead Logging for better concurrency
- **FTS5:** Full-text search virtual table
- **Indexes:** Composite indexes on frequently queried columns
- **Connection Pooling:** Configured via GORM

### Caching Strategy

Currently no application-level caching. Future considerations:
- In-memory cache for frequently accessed tags
- Thumbnail caching with ETags
- Archive metadata caching

### Archive Worker

- Runs in background goroutine
- Polls for pending entries every 30 seconds
- Concurrent job processing with queue

---

## Deployment Architecture

See [ADR-007: Deployment Configuration Patterns](./adr/007-deployment-configuration.md) for detailed deployment documentation.

### Quick Reference

```yaml
# docker-compose.yml (simplified)
services:
  app:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
    environment:
      - SESSION_SECRET=your-secret
      - GO_ENV=production
```

---

## Error Handling

AdHive uses a structured error system with RFC 7807-compatible responses.

### Error Response Format

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "Invalid input: email format",
  "code": "INVALID_EMAIL"
}
```

### Error Codes

| Code | Category | HTTP Status | Description |
|------|----------|-------------|-------------|
| `INVALID_INPUT` | validation | 400 | General input validation error |
| `INVALID_EMAIL` | validation | 400 | Invalid email format |
| `INVALID_PASSWORD` | validation | 400 | Password doesn't meet requirements |
| `UNAUTHORIZED` | auth | 401 | Not authenticated |
| `SESSION_EXPIRED` | auth | 401 | Session has expired |
| `FORBIDDEN` | auth | 403 | Not authorized |
| `NOT_FOUND` | resource | 404 | Resource not found |
| `ENTRY_NOT_FOUND` | resource | 404 | Entry not found |
| `TAG_NOT_FOUND` | resource | 404 | Tag not found |
| `DUPLICATE_ENTRY` | conflict | 409 | Entry already exists |
| `DUPLICATE_USER` | conflict | 409 | User already exists |
| `INTERNAL_ERROR` | internal | 500 | Internal server error |
| `DATABASE_BUSY` | transient | 503 | Database busy, retry |
| `RATE_LIMITED` | transient | 429 | Rate limit exceeded |

---

*Generated: 2026-03-09*