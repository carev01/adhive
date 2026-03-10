# AdHive - Classified Ads Catalog

A self-hosted catalog for organizing and reviewing archived classified advertisements.

## Features

- 📁 **Catalog Management** - Organize ads with tags, notes, and custom fields
- 🔍 **Full-Text Search** - Fast search across titles, descriptions, and notes
- 🎲 **Random Review** - Discover forgotten ads with random selection
- 🖼️ **Thumbnail Generation** - Automatic thumbnail extraction from archived pages
- 📦 **Archive Storage** - Store multiple versions of archived pages
- 🔐 **User Authentication** - Secure session-based authentication

## Quick Start

### Prerequisites

- Go 1.24+ (for building from source)
- Node.js 20+ (for frontend development)
- Docker & Docker Compose (for containerized deployment)

### Development Setup

```bash
# Clone the repository
git clone https://github.com/carev01/adhive.git
cd adhive

# Install dependencies
go mod download
cd frontend && npm install && cd ..

# Start development server
make dev
```

The application will be available at:
- Frontend: http://localhost:5173
- Backend API: http://localhost:8080

## Deployment

AdHive supports two deployment modes: Docker (recommended) and systemd (bare-metal).

### Docker Deployment (Recommended)

#### 1. Create Environment File

```bash
cp .env.example .env
```

Edit `.env` and set required values:

```bash
# REQUIRED for production
SESSION_SECRET=your-secure-secret-here-change-me

# Recommended for production
GO_ENV=production
HTTPS_ENABLED=true
CORS_ALLOWED_ORIGINS=https://yourdomain.com
```

#### 2. Build and Run

```bash
# Build and start
docker compose up -d --build

# View logs
docker compose logs -f app

# Stop
docker compose down
```

#### 3. Data Persistence

Data is stored in `./data` by default (configurable via `DATA_DIR`):

```
data/
├── ad-catalog.db      # SQLite database
├── archives/          # Archived page content
└── thumbnails/        # Generated thumbnails
```

#### Docker Security Features

AdHive containers run with security best practices:

- Non-root user (UID 1000)
- Minimal Alpine Linux base image
- Dropped capabilities (`cap_drop: ALL`)
- No privilege escalation (`no-new-privileges`)
- Resource limits (CPU: 1, Memory: 512MB)

### Systemd Deployment (Bare-Metal)

For servers without Docker, AdHive can run as a systemd service.

#### 1. Build the Binary

```bash
make build
```

#### 2. Run Installation Script

```bash
sudo ./deploy/install-systemd.sh
```

This script:
- Creates `adhive` user and group
- Installs binary to `/opt/adhive/bin/`
- Creates data directory at `/var/lib/adhive/data`
- Installs systemd service unit
- Enables and starts the service

#### 3. Configure

Edit `/etc/adhive/config.env`:

```bash
# Required
SESSION_SECRET=your-secure-secret-here

# Optional
PORT=8080
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=https://yourdomain.com
```

#### 4. Manage Service

```bash
# Check status
sudo systemctl status adhive

# View logs
sudo journalctl -u adhive -f

# Restart
sudo systemctl restart adhive

# Stop
sudo systemctl stop adhive
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `8080` | Server port |
| `DATA_DIR` | No | `./data` | Base data directory |
| `GO_ENV` | No | `development` | Environment mode |
| `LOG_LEVEL` | No | `info` | Logging level |
| `SESSION_SECRET` | Yes (prod) | - | Session signing secret |
| `HTTPS_ENABLED` | No | `false` | Enable HTTPS mode |
| `CORS_ALLOWED_ORIGINS` | No | (dev defaults) | CORS origins |
| `RATE_LIMIT_ENABLED` | No | `true` | Enable rate limiting |
| `RATE_LIMIT_GLOBAL` | No | `100` | Global rate limit/min |
| `RATE_LIMIT_AUTH` | No | `5` | Auth rate limit/min |

### Legacy Variables (Deprecated)

These are supported for backwards compatibility:

| Variable | Notes |
|----------|-------|
| `DB_PATH` | Derived from `DATA_DIR` if not set |
| `STORAGE_DIR` | Derived from `DATA_DIR` if not set |

## Backup & Restore

### Backup

```bash
# Quick backup
./deploy/backup.sh

# Custom location
DATA_DIR=/var/lib/adhive/data BACKUP_DIR=/backups ./deploy/backup.sh
```

### Restore

```bash
# Restore from backup
./deploy/restore.sh /path/to/backup.tar.gz
```

## Development

### Project Structure

```
adhive/
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   ├── auth/            # Authentication logic
│   ├── config/          # Configuration management
│   ├── errors/          # Error types and codes
│   ├── handler/         # HTTP handlers
│   ├── middleware/      # HTTP middleware
│   ├── model/           # Database models
│   ├── repository/      # Data access layer
│   ├── service/         # Business logic
│   └── worker/          # Background workers
├── frontend/            # SvelteKit frontend
├── docs/
│   └── adr/             # Architecture Decision Records
├── deploy/              # Deployment scripts
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

### Make Commands

```bash
make build       # Build binary
make run         # Build and run
make test        # Run tests
make lint        # Run linter
make fmt         # Format code
make docker-up   # Start with Docker
make docker-down # Stop Docker
```

### Architecture Decision Records

Key architectural decisions are documented in `docs/adr/`:

| ADR | Title |
|-----|-------|
| ADR-007 | Deployment Configuration Patterns |

## API

### Health Check

```
GET /health
```

Returns `{"status": "ok"}` if the service is healthy.

### Authentication

```
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/auth/me
```

### Catalog Entries

```
GET    /api/v1/entries          # List entries
POST   /api/v1/entries          # Create entry
GET    /api/v1/entries/:id      # Get entry
PUT    /api/v1/entries/:id      # Update entry
DELETE /api/v1/entries/:id      # Delete entry
GET    /api/v1/entries/random   # Random entry
```

### Tags

```
GET    /api/v1/tags             # List tags
POST   /api/v1/tags             # Create tag
DELETE /api/v1/tags/:id         # Delete tag
```

## Security

### Production Checklist

- [ ] `SESSION_SECRET` set to secure random value
- [ ] `GO_ENV=production`
- [ ] `HTTPS_ENABLED=true` (if using HTTPS)
- [ ] `CORS_ALLOWED_ORIGINS` configured
- [ ] TLS/SSL certificate installed
- [ ] Firewall configured (only port 8080 exposed)
- [ ] Backup strategy implemented

### Report Security Issues

Please report security vulnerabilities to security@yourdomain.com

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

---

*Built with ❤️ by Everton*