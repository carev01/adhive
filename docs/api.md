# AdHive API Documentation

> **Version:** 1.0  
> **Base URL:** `http://localhost:8080/api/v1`

AdHive provides a RESTful API for managing your ad catalog. This document covers all endpoints, authentication, request/response formats, and usage examples.

---

## Table of Contents

1. [Authentication](#authentication)
2. [Error Handling](#error-handling)
3. [Entries](#entries)
4. [Tags](#tags)
5. [Interactions](#interactions)
6. [Archive Operations](#archive-operations)
7. [Files & Storage](#files--storage)
8. [Metrics](#metrics)

---

## Authentication

### Session-Based Auth

AdHive uses session-based authentication via HTTP-only cookies.

| Cookie | Value |
|--------|-------|
| `session` | UUID session token |

**Login Flow:**
1. `POST /api/v1/auth/login` with email + password
2. Server sets `session` cookie (7-day TTL)
3. Include cookie with subsequent requests

**Logout:**
```
POST /api/v1/auth/logout
```

---

## Error Handling

All errors follow [RFC 7807](https://tools.ietf.org/html/rfc7807) Problem Details format:

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "invalid email format"
}
```

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| `200` | Success |
| `201` | Created |
| `204` | No Content (success, no response body) |
| `400` | Bad Request - invalid input |
| `401` | Unauthorized - not logged in |
| `404` | Not Found - resource doesn't exist |
| `409` | Conflict - duplicate resource |
| `500` | Internal Server Error |

---

## Entries

Manage catalog entries (ads).

### List Entries

```
GET /api/v1/entries
```

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | int | 1 | Page number |
| `limit` | int | 20 | Items per page (max 100) |
| `tag` | string | - | Filter by tag ID |
| `status` | string | - | Filter by archive status |
| `search` | string | - | Full-text search |
| `exclude_tried` | bool | false | Exclude entries user tried |

**curl:**
```bash
curl -b session.txt http://localhost:8080/api/v1/entries?page=1&limit=20
```

**Response:**
```json
{
  "entries": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "url": "https://example.com/ad",
      "title": "Ad Title",
      "description": "Ad description",
      "phone_number": "+1234567890",
      "location": "New York, NY",
      "thumbnail_path": "/api/v1/files/thumbnails/uuid",
      "archive_path": "/api/v1/files/archive/uuid",
      "archive_status": "completed",
      "archive_fidelity": "full",
      "tags": [
        { "id": "uuid", "name": "重要", "color": "#FF5733" }
      ],
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T12:00:00Z"
    }
  ],
  "total": 150,
  "page": 1,
  "limit": 20
}
```

---

### Create Entry

```
POST /api/v1/entries
```

**Request Body:**
```json
{
  "url": "https://example.com/ad",
  "title": "Ad Title",
  "description": "Description here",
  "phone_number": "+1234567890",
  "location": "City, State"
}
```

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/entries \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/ad","title":"My Ad"}'
```

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "url": "https://example.com/ad",
  "archive_status": "pending",
  ...
}
```

---

### Get Entry

```
GET /api/v1/entries/:id
```

**curl:**
```bash
curl -b session.txt http://localhost:8080/api/v1/entries/uuid-here
```

---

### Update Entry

```
PUT /api/v1/entries/:id
```

**Request Body (all fields optional):**
```json
{
  "title": "New Title",
  "description": "New description",
  "phone_number": "+9876543210",
  "location": "New Location",
  "thumbnail_path": "/api/v1/files/thumbnails/uuid",
  "archive_path": "/api/v1/files/archive/uuid",
  "archive_status": "completed"
}
```

**curl:**
```bash
curl -b session.txt -X PUT http://localhost:8080/api/v1/entries/uuid-here \
  -H "Content-Type: application/json" \
  -d '{"title":"Updated Title"}'
```

---

### Delete Entry

```
DELETE /api/v1/entries/:id
```

**curl:**
```bash
curl -b session.txt -X DELETE http://localhost:8080/api/v1/entries/uuid-here
```

**Response:** `204 No Content`

---

### Random Entry

```
POST /api/v1/entries/random
```

Get a random entry from the catalog (great for "pick one for me" features).

**Request Body (optional):**
```json
{
  "exclude_tried": true,
  "include_tags": ["tag-uuid"],
  "exclude_tags": ["tag-uuid"]
}
```

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/entries/random \
  -H "Content-Type: application/json" \
  -d '{"exclude_tried": true}'
```

**Response:** `200 OK` — Returns the entry object

---

## Tags

Organize entries with tags.

### List Tags

```
GET /api/v1/tags
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `with_count` | bool | Include entry count per tag |

**curl:**
```bash
curl -b session.txt "http://localhost:8080/api/v1/tags?with_count=true"
```

**Response:**
```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "name": "Important",
    "color": "#FF5733",
    "created_at": "2024-01-15T10:30:00Z"
  }
]
```

With count:
```json
[
  {
    "id": "uuid",
    "user_id": "uuid",
    "name": "Important",
    "color": "#FF5733",
    "created_at": "2024-01-15T10:30:00Z",
    "count": 42
  }
]
```

---

### Create Tag

```
POST /api/v1/tags
```

**Request Body:**
```json
{
  "name": "Important",
  "color": "#FF5733"
}
```

- `name` (required): Max 50 characters
- `color` (optional): Hex color `#RRGGBB`, defaults to `#6B7280`

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/tags \
  -H "Content-Type: application/json" \
  -d '{"name":"Important","color":"#FF5733"}'
```

---

### Update Tag

```
PUT /api/v1/tags/:id
```

**Request Body:**
```json
{
  "name": "New Name",
  "color": "#00FF00"
}
```

---

### Delete Tag

```
DELETE /api/v1/tags/:id
```

**Response:** `204 No Content`

---

### Add Tag to Entry

```
POST /api/v1/entries/:id/tags
```

**Request Body:**
```json
{
  "tag_id": "tag-uuid"
}
```

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/entries/entry-uuid/tags \
  -H "Content-Type: application/json" \
  -d '{"tag_id":"tag-uuid"}'
```

---

### Remove Tag from Entry

```
DELETE /api/v1/entries/:id/tags/:tag_id
```

---

## Interactions

Track user interactions with entries (tried, scored, commented).

### Get Interaction

```
GET /api/v1/entries/:id/interaction
```

**Response:**
```json
{
  "id": "uuid",
  "entry_id": "uuid",
  "user_id": "uuid",
  "tried": true,
  "score": 4,
  "comments": "Good ad, contacted them",
  "contacted_at": "2024-01-20T10:00:00Z",
  "purchased_at": null,
  "created_at": "2024-01-15T12:00:00Z",
  "updated_at": "2024-01-20T10:00:00Z"
}
```

If no interaction exists: `200 OK` with `null` response

---

### Upsert Interaction

```
PUT /api/v1/entries/:id/interaction
```

**Request Body:**
```json
{
  "tried": true,
  "score": 4,
  "comments": "Good ad, contacted them",
  "contacted_at": "2024-01-20T10:00:00Z",
  "purchased_at": null
}
```

**Fields:**
- `tried` (bool): Whether user tried this ad
- `score` (int, 0-5): Rating
- `comments` (string): User notes
- `contacted_at` (ISO 8601): When user contacted the advertiser
- `purchased_at` (ISO 8601): When user made a purchase

**curl:**
```bash
curl -b session.txt -X PUT http://localhost:8080/api/v1/entries/entry-uuid/interaction \
  -H "Content-Type: application/json" \
  -d '{"tried":true,"score":5,"comments":"Great!"}'
```

---

### Delete Interaction

```
DELETE /api/v1/entries/:id/interaction
```

**Response:** `204 No Content`

---

## Archive Operations

Manage web archives and revisions.

### List Revisions

```
GET /api/v1/entries/:id/archive/revisions
```

**Response:**
```json
{
  "revisions": [
    {
      "id": "uuid",
      "entry_id": "uuid",
      "root_path": "data/archives/uuid/rev-xxxx",
      "fidelity": "full",
      "status": "completed",
      "asset_count": 25,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

### Refresh Archive

```
POST /api/v1/entries/:id/archive/refresh
```

Re-processes the entry's URL and creates a new revision.

**Query/Body (optional):**
```
?manual_mode=true
```
or
```json
{
  "manual_mode": true
}
```

**Response:**
```json
{
  "queued": true,
  "manual_mode": false
}
```

---

### Delete Revision

```
DELETE /api/v1/entries/:id/archive/revisions/:revisionId
```

Deletes a specific revision from both DB and disk.

**Response:**
```json
{
  "deleted": true
}
```

---

## Files & Storage

### Upload Archive

```
POST /api/v1/files/archives/:entryID
```

Upload a ZIP archive for an entry.

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/files/archives/entry-uuid \
  -F "archive=@/path/to/archive.zip"
```

---

### List Archives

```
GET /api/v1/files/archives/:entryID
```

---

### Get Archive

```
GET /api/v1/files/archive/:entryID
GET /api/v1/files/archive/:entryID/:revisionID/*path
```

Serves archived web content with security headers.

---

### Upload Thumbnail

```
POST /api/v1/files/thumbnails/:entryID
```

**curl:**
```bash
curl -b session.txt -X POST http://localhost:8080/api/v1/files/thumbnails/entry-uuid \
  -F "thumbnail=@/path/to/image.jpg"
```

---

### Get Thumbnail

```
GET /api/v1/files/thumbnails/:entryID
```

---

### Delete Thumbnail

```
DELETE /api/v1/files/thumbnails/:entryID
```

---

### Thumbnail Candidates

```
GET /api/v1/files/thumbnails/:entryID/candidates
```

List auto-generated thumbnail candidates.

```
POST /api/v1/files/thumbnails/:entryID/select
```

Select a candidate as the entry's thumbnail.

**Request Body:**
```json
{
  "candidate_id": "candidate-uuid"
}
```

---

### Storage Stats

```
GET /api/v1/files/stats
```

Returns storage usage statistics.

---

## Metrics

### Archive Metrics

```
GET /api/v1/archive/metrics
```

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hours` | int | 24 | Time window in hours (1-720) |

**Response:**
```json
{
  "window_hours": 24,
  "since": "2024-01-14T10:30:00Z",
  "metrics": {
    "total_revisions": 150,
    "completed_count": 120,
    "partial_count": 20,
    "blocked_count": 5,
    "failed_count": 5,
    "total_assets": 4500,
    "total_bytes": 5368709120
  },
  "partial_rate": 0.133,
  "blocked_rate": 0.033,
  "failed_rate": 0.033,
  "average_asset_bytes": 1193046
}
```

---

## Quick Start Example

```bash
# 1. Register a new user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"securepassword123","display_name":"John"}' \
  -c session.txt

# 2. Create a tag
curl -b session.txt -X POST http://localhost:8080/api/v1/tags \
  -H "Content-Type: application/json" \
  -d '{"name":"Important","color":"#FF5733"}'

# 3. Add an entry
curl -b session.txt -X POST http://localhost:8080/api/v1/entries \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/classified-ad","title":"Great Deal"}'

# 4. List entries
curl -b session.txt http://localhost:8080/api/v1/entries

# 5. Get a random entry
curl -b session.txt -X POST http://localhost:8080/api/v1/entries/random \
  -H "Content-Type: application/json" \
  -d '{}'

# 6. Logout
curl -b session.txt -X POST http://localhost:8080/api/v1/auth/logout
```

---

## Archive Status Values

| Status | Description |
|--------|-------------|
| `pending` | Waiting to be archived |
| `in_progress` | Currently archiving |
| `completed` | Successfully archived |
| `partial` | Partially archived (some resources blocked) |
| `blocked` | Entirely blocked (paywall, robots.txt) |
| `failed` | Archive failed |

---

## Fidelity Levels

| Fidelity | Description |
|----------|-------------|
| `full` | Complete page capture with all assets |
| `simplified` | HTML-only, no external assets |
| `screenshot` | Visual screenshot only |

---

*Generated for AdHive v1.0*
