# AdHive API Reference

**Version:** 1.0.0  
**Base URL:** `/api/v1`

---

## Overview

AdHive provides a RESTful API for managing a catalog of classified advertisements. All endpoints require authentication unless otherwise noted.

### Authentication

AdHive uses session-based authentication with HTTP-only cookies.

| Cookie | Description |
|--------|-------------|
| `session` | Session token (HTTP-only, Secure, SameSite=Strict) |
| `csrf_token` | CSRF token for state-changing requests |

### Content Types

- **Request:** `application/json`
- **Response:** `application/json`

### Error Response Format

All errors follow RFC 7807 Problem Details format:

```json
{
  "type": "about:blank",
  "title": "Error Title",
  "status": 400,
  "detail": "Detailed error message",
  "code": "ERROR_CODE"
}
```

---

## Endpoints

### Authentication

#### Register User

```
POST /api/v1/auth/register
```

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "securepassword123",
  "display_name": "John Doe"
}
```

**Response:** `201 Created`
```json
{
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "display_name": "John Doe",
    "created_at": "2026-03-09T12:00:00Z"
  }
}
```

**Error Codes:**
- `INVALID_INPUT` - Missing or invalid fields
- `INVALID_EMAIL` - Email format invalid
- `INVALID_PASSWORD` - Password too short (min 8 chars)
- `DUPLICATE_USER` - Email already registered

---

#### Login

```
POST /api/v1/auth/login
```

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Response:** `200 OK`
```json
{
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "display_name": "John Doe",
    "created_at": "2026-03-09T12:00:00Z"
  }
}
```

**Error Codes:**
- `UNAUTHORIZED` - Invalid email or password
- `FORBIDDEN` - Account is disabled

---

#### Logout

```
POST /api/v1/auth/logout
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "message": "logged out"
}
```

---

#### Get Current User

```
GET /api/v1/auth/me
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "display_name": "John Doe",
  "created_at": "2026-03-09T12:00:00Z"
}
```

---

#### Get CSRF Token

```
GET /api/v1/auth/csrf-token
```

**Response:** `200 OK`
```json
{
  "csrf_token": "uuid-token"
}
```

---

### Entries

#### List Entries

```
GET /api/v1/entries
```

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number |
| `limit` | integer | 20 | Items per page (max 100) |
| `tag` | string | - | Filter by tag ID |
| `status` | string | - | Filter by archive status |
| `exclude_tried` | boolean | false | Exclude entries marked as tried |
| `search` | string | - | Full-text search query |
| `date_from` | string | - | Filter from date (YYYY-MM-DD) |
| `date_to` | string | - | Filter to date (YYYY-MM-DD) |
| `source` | string | - | Filter by source domain |
| `location` | string | - | Filter by location |
| `sort_by` | string | created_at | Sort field |
| `sort_order` | string | desc | Sort order (asc/desc) |
| `has_interaction` | boolean | false | Only entries with interactions |
| `min_score` | integer | 0 | Minimum interaction score |

**Response:** `200 OK`
```json
{
  "entries": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "url": "https://example.com/ad/123",
      "title": "Ad Title",
      "description": "Ad description",
      "phone_number": "+1-555-123-4567",
      "location": "New York, NY",
      "thumbnail_path": "/api/v1/files/thumbnails/uuid",
      "archive_path": "/api/v1/files/archive/uuid",
      "archive_status": "completed",
      "archive_fidelity": "high",
      "archive_current_revision_id": "uuid",
      "thumbnail_source": "auto",
      "tags": [
        {"id": "uuid", "name": "Electronics", "color": "#3B82F6"}
      ],
      "created_at": "2026-03-09T12:00:00Z",
      "updated_at": "2026-03-09T12:00:00Z"
    }
  ],
  "total": 100,
  "page": 1,
  "limit": 20
}
```

---

#### Create Entry

```
POST /api/v1/entries
```

**Authentication:** Required

**Request Body:**
```json
{
  "url": "https://example.com/ad/123",
  "title": "Ad Title",
  "description": "Ad description",
  "phone_number": "+1-555-123-4567",
  "location": "New York, NY"
}
```

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "url": "https://example.com/ad/123",
  "title": "Ad Title",
  "description": "Ad description",
  "phone_number": "+1-555-123-4567",
  "location": "New York, NY",
  "archive_status": "pending",
  "tags": [],
  "created_at": "2026-03-09T12:00:00Z",
  "updated_at": "2026-03-09T12:00:00Z"
}
```

---

#### Get Entry

```
GET /api/v1/entries/:id
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "url": "https://example.com/ad/123",
  "title": "Ad Title",
  "description": "Ad description",
  "phone_number": "+1-555-123-4567",
  "location": "New York, NY",
  "thumbnail_path": "/api/v1/files/thumbnails/uuid",
  "archive_path": "/api/v1/files/archive/uuid",
  "archive_status": "completed",
  "archive_fidelity": "high",
  "archive_current_revision_id": "uuid",
  "thumbnail_source": "auto",
  "tags": [
    {"id": "uuid", "name": "Electronics", "color": "#3B82F6"}
  ],
  "created_at": "2026-03-09T12:00:00Z",
  "updated_at": "2026-03-09T12:00:00Z"
}
```

**Error Codes:**
- `ENTRY_NOT_FOUND` - Entry not found

---

#### Update Entry

```
PUT /api/v1/entries/:id
```

**Authentication:** Required

**Request Body:**
```json
{
  "title": "Updated Title",
  "description": "Updated description",
  "phone_number": "+1-555-987-6543",
  "location": "Los Angeles, CA",
  "thumbnail_path": "/api/v1/files/thumbnails/uuid",
  "archive_path": "/api/v1/files/archive/uuid",
  "archive_status": "completed"
}
```

**Response:** `200 OK`

---

#### Delete Entry

```
DELETE /api/v1/entries/:id
```

**Authentication:** Required

**Response:** `204 No Content`

---

#### Get Random Entry

```
POST /api/v1/entries/random
```

**Authentication:** Required

**Request Body:**
```json
{
  "exclude_tried": true,
  "include_tags": ["tag-uuid-1", "tag-uuid-2"],
  "exclude_tags": ["tag-uuid-3"]
}
```

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "url": "https://example.com/ad/123",
  "title": "Random Ad",
  "...": "..."
}
```

**Error Codes:**
- `NOT_FOUND` - No entries found matching criteria

---

#### Get Sources

```
GET /api/v1/entries/sources
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "sources": ["example.com", "another-site.com", "marketplace.org"]
}
```

---

#### Get Locations

```
GET /api/v1/entries/locations
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "locations": ["New York, NY", "Los Angeles, CA", "Chicago, IL"]
}
```

---

#### Bulk Tag Entries

```
POST /api/v1/entries/bulk/tag
```

**Authentication:** Required

**Request Body:**
```json
{
  "entry_ids": ["uuid-1", "uuid-2", "uuid-3"],
  "tag_ids": ["tag-uuid-1", "tag-uuid-2"],
  "action": "add"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "action": "add",
  "count": 3
}
```

---

#### Bulk Delete Entries

```
POST /api/v1/entries/bulk/delete
```

**Authentication:** Required

**Request Body:**
```json
{
  "entry_ids": ["uuid-1", "uuid-2", "uuid-3"]
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "deleted": 3
}
```

---

#### Bulk Archive Entries

```
POST /api/v1/entries/bulk/archive
```

**Authentication:** Required

**Request Body:**
```json
{
  "entry_ids": ["uuid-1", "uuid-2", "uuid-3"]
}
```

**Response:** `202 Accepted`
```json
{
  "queued": 3
}
```

---

### Tags

#### List Tags

```
GET /api/v1/tags
```

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `with_count` | boolean | false | Include entry counts |

**Response:** `200 OK`
```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "name": "Electronics",
    "color": "#3B82F6",
    "created_at": "2026-03-09T12:00:00Z"
  }
]
```

---

#### Create Tag

```
POST /api/v1/tags
```

**Authentication:** Required

**Request Body:**
```json
{
  "name": "Electronics",
  "color": "#3B82F6"
}
```

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "Electronics",
  "color": "#3B82F6",
  "created_at": "2026-03-09T12:00:00Z"
}
```

---

#### Get Tag

```
GET /api/v1/tags/:id
```

**Authentication:** Required

**Response:** `200 OK`

---

#### Update Tag

```
PUT /api/v1/tags/:id
```

**Authentication:** Required

**Request Body:**
```json
{
  "name": "Updated Name",
  "color": "#10B981"
}
```

**Response:** `200 OK`

---

#### Delete Tag

```
DELETE /api/v1/tags/:id
```

**Authentication:** Required

**Response:** `204 No Content`

---

#### Add Tag to Entry

```
POST /api/v1/entries/:id/tags
```

**Authentication:** Required

**Request Body:**
```json
{
  "tag_id": "tag-uuid"
}
```

**Response:** `200 OK`
```json
{
  "message": "tag added to entry"
}
```

---

#### Remove Tag from Entry

```
DELETE /api/v1/entries/:id/tags/:tagId
```

**Authentication:** Required

**Response:** `204 No Content`

---

### Interactions

#### Get Interaction

```
GET /api/v1/entries/:id/interaction
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "entry_id": "uuid",
  "user_id": "uuid",
  "tried": true,
  "score": 4,
  "comments": "Great seller, fast shipping",
  "contacted_at": "2026-03-09T12:00:00Z",
  "purchased_at": "2026-03-10T15:30:00Z",
  "created_at": "2026-03-09T12:00:00Z",
  "updated_at": "2026-03-10T15:30:00Z"
}
```

---

#### Upsert Interaction

```
PUT /api/v1/entries/:id/interaction
```

**Authentication:** Required

**Request Body:**
```json
{
  "tried": true,
  "score": 4,
  "comments": "Great seller, fast shipping",
  "contacted_at": "2026-03-09T12:00:00Z",
  "purchased_at": "2026-03-10T15:30:00Z"
}
```

**Response:** `200 OK`

---

#### Delete Interaction

```
DELETE /api/v1/entries/:id/interaction
```

**Authentication:** Required

**Response:** `204 No Content`

---

### Archive Operations

#### List Revisions

```
GET /api/v1/entries/:id/archive/revisions
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "revisions": [
    {
      "id": "uuid",
      "entry_id": "uuid",
      "captured_at": "2026-03-09T12:00:00Z",
      "root_path": "data/archives/uuid/rev-0001",
      "fidelity": "high",
      "notes": "",
      "created_at": "2026-03-09T12:00:00Z"
    }
  ]
}
```

---

#### Refresh Archive

```
POST /api/v1/entries/:id/archive/refresh
```

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `manual_mode` | boolean | Enable manual capture mode |

**Request Body:**
```json
{
  "manual_mode": true
}
```

**Response:** `202 Accepted`
```json
{
  "queued": true,
  "manual_mode": true
}
```

---

#### Delete Revision

```
DELETE /api/v1/entries/:id/archive/revisions/:revisionId
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "deleted": true
}
```

---

#### Get Archive Metrics

```
GET /api/v1/archive/metrics
```

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hours` | integer | 24 | Time window in hours |

**Response:** `200 OK`
```json
{
  "window_hours": 24,
  "since": "2026-03-08T12:00:00Z",
  "metrics": {
    "total_revisions": 50,
    "partial_count": 5,
    "blocked_count": 2,
    "failed_count": 1,
    "total_assets": 500,
    "total_bytes": 10485760
  },
  "partial_rate": 0.1,
  "blocked_rate": 0.04,
  "failed_rate": 0.02,
  "average_asset_bytes": 20971
}
```

---

### Thumbnails

#### List Thumbnail Candidates

```
GET /api/v1/files/thumbnails/:entryID/candidates
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "candidates": [
    {
      "id": "uuid",
      "entry_id": "uuid",
      "revision_id": "uuid",
      "source_type": "local_asset",
      "path": "/api/v1/files/thumbnails/raw/uuid/rev-0001/image.webp",
      "score": 0.85,
      "selected": true
    }
  ]
}
```

---

#### Select Thumbnail Candidate

```
POST /api/v1/files/thumbnails/:entryID/select
```

**Authentication:** Required

**Request Body:**
```json
{
  "candidate_id": "uuid"
}
```

**Response:** `200 OK`
```json
{
  "thumbnail_path": "/api/v1/files/thumbnails/uuid",
  "thumbnail_source": "user_selected",
  "selected_candidate_id": "uuid"
}
```

---

### Files

#### Get Archive

```
GET /api/v1/files/archive/:entryID
GET /api/v1/files/archive/:entryID/:revisionID/*path
```

**Authentication:** Required

**Response:** File content with appropriate Content-Type

---

#### Upload Archive

```
POST /api/v1/files/archives/:entryID
```

**Authentication:** Required

**Request:** `multipart/form-data` with archive file

**Response:** `200 OK`

---

#### Get Thumbnail

```
GET /api/v1/files/thumbnails/:entryID
```

**Authentication:** Required

**Response:** Image file (WebP)

---

#### Upload Thumbnail

```
POST /api/v1/files/thumbnails/:entryID
```

**Authentication:** Required

**Request:** `multipart/form-data` with image file

**Response:** `200 OK`

---

#### Get Storage Stats

```
GET /api/v1/files/stats
```

**Authentication:** Required

**Response:** `200 OK`
```json
{
  "archives_size": 1073741824,
  "thumbnails_size": 52428800,
  "total_size": 1126170624,
  "archives_count": 100,
  "thumbnails_count": 100
}
```

---

## Health Check

```
GET /health
```

**Response:** `200 OK`
```json
{
  "status": "ok"
}
```

---

## Version Info

```
GET /version
```

**Response:** `200 OK`
```json
{
  "version": "1.0.0",
  "commit": "abc123",
  "build_date": "2026-03-09T12:00:00Z"
}
```

---

## Error Codes Reference

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_INPUT` | 400 | General input validation error |
| `INVALID_EMAIL` | 400 | Invalid email format |
| `INVALID_PASSWORD` | 400 | Password doesn't meet requirements |
| `MISSING_FIELD` | 400 | Required field missing |
| `INVALID_FORMAT` | 400 | Invalid format (e.g., color hex) |
| `UNAUTHORIZED` | 401 | Not authenticated |
| `SESSION_EXPIRED` | 401 | Session has expired |
| `FORBIDDEN` | 403 | Not authorized for this resource |
| `NOT_FOUND` | 404 | Resource not found |
| `ENTRY_NOT_FOUND` | 404 | Entry not found |
| `TAG_NOT_FOUND` | 404 | Tag not found |
| `USER_NOT_FOUND` | 404 | User not found |
| `DUPLICATE_ENTRY` | 409 | Entry already exists |
| `DUPLICATE_TAG` | 409 | Tag already exists |
| `DUPLICATE_USER` | 409 | User already exists |
| `INTERNAL_ERROR` | 500 | Internal server error |
| `PLAYWRIGHT_FAILED` | 502 | Playwright capture failed |
| `ARCHIVE_FAILED` | 502 | Archive operation failed |
| `THUMBNAIL_FAILED` | 502 | Thumbnail generation failed |
| `EXTERNAL_SERVICE_ERROR` | 502 | External service error |
| `DATABASE_BUSY` | 503 | Database busy, please retry |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `TIMEOUT` | 504 | Request timeout |
| `TEMPORARY_FAILURE` | 503 | Temporary failure, please retry |

---

## Rate Limits

| Endpoint Type | Limit | Window |
|---------------|-------|--------|
| Global | 100 requests | 1 minute |
| Auth (login) | 5 requests | 1 minute |
| Auth (register) | 3 requests | 1 hour |

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1709992800
```

---

*Generated: 2026-03-09*