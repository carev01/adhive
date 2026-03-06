# AdHive

A classified ads catalog system for organizing, tracking, and managing online classified advertisements.

## Quick Start

### Development

```bash
# Install dependencies
make deps

# Build
make build

# Run
make run

# Or with Docker
make docker-up
```

### Docker

```bash
# Build image
make docker-build

# Run container
make docker-run
```

## Project Structure

```
adhive/
├── cmd/server/          # Application entrypoint
├── internal/            # Internal packages
│   ├── config/          # Configuration
│   ├── handler/         # HTTP handlers
│   ├── middleware/      # Middleware
│   ├── model/           # Data models
│   ├── repository/      # Data access
│   └── service/         # Business logic
├── pkg/                 # Shared packages
├── migrations/          # Database migrations
├── frontend/            # SvelteKit frontend
├── Dockerfile          # Container definition
├── docker-compose.yml   # Local dev environment
└── Makefile            # Build tasks
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8080 | Server port |
| DATA_DIR | ./data | Data directory |
| LOG_LEVEL | info | Logging level |

## API Endpoints

- `GET /` - Root info
- `GET /health` - Health check
- `GET /healthz` - Liveness probe

## Tech Stack

- Backend: Go + Gin
- Frontend: SvelteKit
- Database: SQLite
- Container: Docker