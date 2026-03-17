# ADR-007: Deployment Configuration Patterns

**Status:** Proposed  
**Date:** 2026-03-09  
**Decision Makers:** @bumblebee, @jarvis  
**Related ADRs:** ADR-005 (Security Hardening), ADR-006 (Remaining Security)

---

## Executive Summary

This ADR defines deployment configuration patterns for AdHive, addressing:
1. Unified data directory configuration
2. Docker deployment (non-root user, security)
3. Non-Docker deployment (systemd service, binary distribution)
4. Environment configuration (required vs optional)
5. Security considerations for data paths

---

## Current State Analysis

### Configuration Variables

| Variable | Default | Description | Issue |
|----------|---------|-------------|-------|
| `PORT` | `8080` | Server port | OK |
| `DB_PATH` | `ad-catalog.db` | Database file path | Relative, inconsistent |
| `STORAGE_DIR` | `./data` | File storage base dir | Relative, inconsistent |
| `LOG_LEVEL` | `info` | Logging level | OK |
| `USER_AGENT` | (auto) | Scraper user agent | OK |
| `HUMAN_DOMAINS` | (empty) | Manual capture domains | OK |
| `CORS_ALLOWED_ORIGINS` | (empty) | CORS origins | Missing defaults |

### Dockerfile Issues

```dockerfile
# Current Dockerfile issues:
FROM scratch  # No shell, no user management
RUN mkdir -p /app/data /app/logs  # Fails in scratch!
ENV DATA_DIR=/app/data  # Inconsistent with STORAGE_DIR in code
```

**Problems:**
1. `FROM scratch` doesn't support `RUN mkdir` - should use `COPY` from builder
2. `DATA_DIR` env var doesn't match `STORAGE_DIR` in Go code
3. No non-root user setup
4. No healthcheck binary in scratch image

### docker-compose.yml Issues

```yaml
# Current volume setup:
volumes:
  - ad-catalog-data:/app/data  # Named volume
```

**Problems:**
1. Named volume makes backup/restore harder
2. Database file and storage in same location (OK for dev, problematic for production)
3. No backup strategy defined

### Missing Configuration

1. **SESSION_SECRET** - Required for secure sessions
2. **HTTPS_ENABLED** - For HSTS and cookie security
3. **RATE_LIMITS** - Auth and global rate limiting
4. **CORS defaults** - Development vs production defaults

---

## Decision

### 1. Unified Data Directory Configuration

**Problem:** `DB_PATH` and `STORAGE_DIR` are separate, can diverge.

**Solution:** Introduce `DATA_DIR` as the unified base, with derived paths.

```go
// internal/config/config.go

package config

import (
    "os"
    "path/filepath"
)

// Config holds all application configuration
type Config struct {
    // Server
    Port    string
    
    // Data paths
    DataDir  string
    DBPath   string
    
    // Storage paths (derived from DataDir)
    ArchivesDir    string
    ThumbnailsDir  string
    
    // Security
    SessionSecret   string
    SessionTTL      string
    HTTPSEnabled    bool
    CookieSecure    bool
    
    // CORS
    CORSAllowedOrigins []string
    
    // Rate limiting
    RateLimitGlobal  int
    RateLimitAuth    int
    
    // Environment
    Environment string  // "development" or "production"
    LogLevel    string
}

// Load reads configuration from environment
func Load() *Config {
    cfg := &Config{
        Port:         getEnv("PORT", "8080"),
        DataDir:      getEnv("DATA_DIR", "./data"),
        LogLevel:     getEnv("LOG_LEVEL", "info"),
        Environment:  getEnv("GO_ENV", "development"),
    }
    
    // Derive paths from DATA_DIR
    cfg.DBPath = filepath.Join(cfg.DataDir, "ad-catalog.db")
    cfg.ArchivesDir = filepath.Join(cfg.DataDir, "archives")
    cfg.ThumbnailsDir = filepath.Join(cfg.DataDir, "thumbnails")
    
    // Security
    cfg.SessionSecret = getEnv("SESSION_SECRET", "")
    cfg.HTTPSEnabled = getEnv("HTTPS_ENABLED", "false") == "true"
    cfg.CookieSecure = cfg.HTTPSEnabled // Secure cookies only with HTTPS
    
    // CORS
    cfg.CORSAllowedOrigins = parseCORSOrigins()
    
    // Rate limits
    cfg.RateLimitGlobal = getEnvInt("RATE_LIMIT_GLOBAL", 100)
    cfg.RateLimitAuth = getEnvInt("RATE_LIMIT_AUTH", 5)
    
    return cfg
}

// StorageConfig returns storage configuration derived from main config
func (c *Config) StorageConfig() *StorageConfig {
    return &StorageConfig{
        BaseDir:     c.DataDir,
        ArchivesDir: c.ArchivesDir,
        ThumbDir:    c.ThumbnailsDir,
    }
}

// DatabaseConfig returns database configuration
func (c *Config) DatabaseConfig() *DatabaseConfig {
    return &DatabaseConfig{
        Path: c.DBPath,
    }
}

func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
    if val := os.Getenv(key); val != "" {
        // parse int...
    }
    return defaultVal
}

func parseCORSOrigins() []string {
    envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
    if envOrigins == "" {
        if os.Getenv("GO_ENV") == "development" {
            return []string{
                "http://localhost:5173",
                "http://localhost:3000",
                "http://localhost:8080",
            }
        }
        return []string{}
    }
    // parse comma-separated...
}
```

**Environment Variable Mapping:**

| Variable | Old | New | Notes |
|----------|-----|-----|-------|
| `DATA_DIR` | N/A | `./data` | **New unified base** |
| `DB_PATH` | `ad-catalog.db` | (derived from DATA_DIR) | Backwards compatible |
| `STORAGE_DIR` | `./data` | (derived from DATA_DIR) | Backwards compatible |

**Backwards Compatibility:**

```go
// Support both old and new configuration
func getDataDir() string {
    // New unified config
    if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
        return dataDir
    }
    
    // Legacy: derive from DB_PATH and STORAGE_DIR
    dbPath := os.Getenv("DB_PATH")
    storageDir := os.Getenv("STORAGE_DIR")
    
    if storageDir != "" {
        return storageDir
    }
    
    if dbPath != "" {
        return filepath.Dir(dbPath)
    }
    
    return "./data"
}
```

---

### 2. Docker Deployment (Security Hardened)

**Problem:** Current Dockerfile uses `FROM scratch`, no non-root user, inconsistent config.

**Solution:** Multi-stage build with non-root user.

```dockerfile
# Dockerfile
# syntax=docker/dockerfile:1

# ============================================
# Build stage
# ============================================
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY migrations/ ./migrations/

# Build arguments for version info
ARG VERSION=latest
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the binary with version info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w \
        -X main.version=${VERSION} \
        -X main.commit=${COMMIT} \
        -X main.buildDate=${BUILD_DATE}" \
    -o ad-catalog ./cmd/server

# ============================================
# Frontend build stage (optional, for production)
# ============================================
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder

WORKDIR /app

# Copy frontend source
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# ============================================
# Final stage - minimal runtime image
# ============================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S adhive && \
    adduser -u 1000 -S -G adhive -s /bin/sh adhive

# Create directories with proper permissions
RUN mkdir -p /app/data /app/logs && \
    chown -R adhive:adhive /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/ad-catalog /app/ad-catalog
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy frontend build (if exists)
COPY --from=frontend-builder --chown=adhive:adhive /app/build/ /app/frontend/ 2>/dev/null || true

# Switch to non-root user
USER adhive

# Environment defaults
ENV PORT=8080 \
    DATA_DIR=/app/data \
    GO_ENV=production \
    LOG_LEVEL=info

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/ad-catalog"]
```

**Key Security Improvements:**

1. **Non-root user:** `adhive` user (UID 1000)
2. **Minimal image:** Alpine Linux (smaller attack surface)
3. **Health check:** Built-in health monitoring
4. **Proper permissions:** Directories owned by non-root user

---

### 3. Docker Compose (Production-Ready)

```yaml
# docker-compose.yml
version: '3.8'

services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        VERSION: ${VERSION:-latest}
        COMMIT: ${COMMIT:-unknown}
        BUILD_DATE: ${BUILD_DATE:-unknown}
    container_name: adhive-app
    restart: unless-stopped
    
    # Security: Run as non-root user
    user: "1000:1000"
    
    ports:
      - "${PORT:-8080}:8080"
    
    environment:
      - PORT=8080
      - DATA_DIR=/app/data
      - GO_ENV=production
      - LOG_LEVEL=${LOG_LEVEL:-info}
      - SESSION_SECRET=${SESSION_SECRET:?SESSION_SECRET is required}
      - HTTPS_ENABLED=${HTTPS_ENABLED:-false}
      - CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS:-}
      - RATE_LIMIT_GLOBAL=${RATE_LIMIT_GLOBAL:-100}
      - RATE_LIMIT_AUTH=${RATE_LIMIT_AUTH:-5}
    
    volumes:
      # Use bind mount for easy backup/restore
      - ${DATA_DIR:-./data}:/app/data:Z
    
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    
    # Security: Drop all capabilities, add only what's needed
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE  # If binding to port < 1024
    
    # Security: No privilege escalation
    security_opt:
      - no-new-privileges:true
    
    # Resource limits
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 128M

  # Optional: Frontend development server
  frontend:
    image: node:20-alpine
    container_name: adhive-frontend
    working_dir: /app
    ports:
      - "${FRONTEND_PORT:-5173}:5173"
    volumes:
      - ./frontend:/app
      - /app/node_modules
    environment:
      - VITE_API_URL=${VITE_API_URL:-http://localhost:8080}
    command: sh -c "npm install && npm run dev -- --host"
    profiles:
      - dev

networks:
  default:
    name: adhive-network
```

**Key Improvements:**

1. **Bind mount:** Easier backup/restore than named volumes
2. **Resource limits:** Prevent resource exhaustion
3. **Security options:** Drop capabilities, no-new-privileges
4. **Required secrets:** `SESSION_SECRET` must be set
5. **Development profile:** Frontend only runs with `--profile dev`

---

### 4. Non-Docker Deployment (Systemd)

**Problem:** No systemd service file for bare-metal deployments.

**Solution:** Create systemd service unit.

```ini
# /etc/systemd/system/adhive.service

[Unit]
Description=AdHive - Classified Ads Catalog
Documentation=https://docs.adhive.ai
After=network.target

[Service]
Type=notify
User=adhive
Group=adhive
WorkingDirectory=/opt/adhive

# Environment
Environment=PORT=8080
Environment=DATA_DIR=/var/lib/adhive/data
Environment=GO_ENV=production
Environment=LOG_LEVEL=info
EnvironmentFile=/etc/adhive/config.env

# Binary
ExecStart=/opt/adhive/bin/ad-catalog
ExecReload=/bin/kill -HUP $MAINPID

# Restart policy
Restart=on-failure
RestartSec=5s

# Security hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/adhive/data
ReadOnlyPaths=/etc/adhive

# Resource limits
LimitNOFILE=65535
MemoryMax=512M
CPUQuota=1

[Install]
WantedBy=multi-user.target
```

**Installation Script:**

```bash
#!/bin/bash
# install-systemd.sh

set -e

# Create user and group
sudo useradd --system --home-dir /var/lib/adhive --shell /usr/sbin/nologin adhive || true

# Create directories
sudo mkdir -p /opt/adhive/bin
sudo mkdir -p /var/lib/adhive/data
sudo mkdir -p /etc/adhive

# Copy binary
sudo cp bin/ad-catalog /opt/adhive/bin/
sudo chmod +x /opt/adhive/bin/ad-catalog

# Copy systemd unit
sudo cp deploy/adhive.service /etc/systemd/system/

# Create config file
if [ ! -f /etc/adhive/config.env ]; then
    sudo cp deploy/config.env.example /etc/adhive/config.env
    echo "Please edit /etc/adhive/config.env with your configuration"
fi

# Set permissions
sudo chown -R adhive:adhive /var/lib/adhive
sudo chmod 750 /var/lib/adhive
sudo chmod 750 /var/lib/adhive/data

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable adhive
sudo systemctl start adhive

echo "AdHive installed successfully!"
echo "Check status with: sudo systemctl status adhive"
echo "View logs with: sudo journalctl -u adhive -f"
```

---

### 5. Environment Configuration

**Problem:** Incomplete documentation, missing required variables.

**Solution:** Comprehensive environment configuration.

```bash
# .env.example (updated)

# ===========================================
# AdHive Configuration
# ===========================================
# Copy this file to .env for Docker Compose
# For systemd, copy to /etc/adhive/config.env

# ===========================================
# REQUIRED (Production)
# ===========================================

# Session secret for secure cookies
# Generate with: openssl rand -hex 32
SESSION_SECRET=your-secret-here-change-me

# ===========================================
# Server Configuration
# ===========================================

# Port the server listens on
PORT=8080

# Environment: development or production
GO_ENV=development

# ===========================================
# Data Configuration
# ===========================================

# Base directory for all data (database, archives, thumbnails)
# Docker: /app/data
# Bare metal: /var/lib/adhive/data
DATA_DIR=./data

# Legacy support (derived from DATA_DIR if not set)
# DB_PATH=./data/ad-catalog.db
# STORAGE_DIR=./data

# ===========================================
# Security Configuration
# ===========================================

# Enable HTTPS (affects cookie security and HSTS)
HTTPS_ENABLED=false

# CORS allowed origins (comma-separated)
# Development defaults to localhost ports
# Production: must be explicitly configured
# CORS_ALLOWED_ORIGINS=http://localhost:5173,https://yourdomain.com

# ===========================================
# Rate Limiting
# ===========================================

# Global rate limit (requests per minute)
RATE_LIMIT_GLOBAL=100

# Auth rate limit (requests per minute)
RATE_LIMIT_AUTH=5

# ===========================================
# Logging
# ===========================================

# Log level: debug, info, warn, error
LOG_LEVEL=info

# ===========================================
# Scraper Configuration (Optional)
# ===========================================

# Override Chrome User-Agent
# USER_AGENT=

# Domains requiring manual intervention (comma-separated)
# HUMAN_DOMAINS=captcha-heavy-site.com,another-site.com
```

**Production Checklist:**

```markdown
## Production Deployment Checklist

### Required Configuration
- [ ] SESSION_SECRET set to secure random value
- [ ] GO_ENV=production
- [ ] HTTPS_ENABLED=true (if using HTTPS)
- [ ] CORS_ALLOWED_ORIGINS configured

### Security
- [ ] Non-root user running the service
- [ ] File permissions correct (750 for data dir)
- [ ] TLS/SSL certificate installed (if HTTPS)
- [ ] Firewall configured (only port 8080 exposed)

### Backup
- [ ] Backup strategy for DATA_DIR
- [ ] Database backup scheduled
- [ ] Archive backup scheduled

### Monitoring
- [ ] Health check endpoint accessible
- [ ] Log aggregation configured
- [ ] Alerting configured for errors
```

---

### 6. Backup Strategy

**Problem:** No backup strategy defined.

**Solution:** Define backup approach for data directory.

```bash
#!/bin/bash
# backup.sh

set -e

DATA_DIR="${DATA_DIR:-./data}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/adhive-backup-${DATE}.tar.gz"

# Create backup directory
mkdir -p "${BACKUP_DIR}"

# Stop service for consistent backup (optional)
# systemctl stop adhive

# Create backup
tar -czf "${BACKUP_FILE}" \
    --exclude="*.log" \
    --exclude="*.tmp" \
    -C "$(dirname "${DATA_DIR}")" \
    "$(basename "${DATA_DIR}")"

# Restart service
# systemctl start adhive

# Clean old backups (keep last 7)
find "${BACKUP_DIR}" -name "adhive-backup-*.tar.gz" -mtime +7 -delete

echo "Backup created: ${BACKUP_FILE}"
```

**Restore Script:**

```bash
#!/bin/bash
# restore.sh

set -e

DATA_DIR="${DATA_DIR:-./data}"
BACKUP_FILE="${1:?Usage: restore.sh <backup-file>}"

# Stop service
systemctl stop adhive

# Backup current data
mv "${DATA_DIR}" "${DATA_DIR}.old"

# Restore
tar -xzf "${BACKUP_FILE}" -C /

# Start service
systemctl start adhive

echo "Restore complete from ${BACKUP_FILE}"
```

---

## Migration Path

### From Current to New Configuration

**Step 1: Update Configuration Files**

1. Update `.env.example` with new variables
2. Update `config/storage.go` to use `DATA_DIR`
3. Update `cmd/server/main.go` to use unified config

**Step 2: Update Dockerfile**

1. Replace `FROM scratch` with `FROM alpine`
2. Add non-root user
3. Fix directory creation
4. Add health check

**Step 3: Update docker-compose.yml**

1. Add security options
2. Add resource limits
3. Update environment variables

**Step 4: Add Systemd Support**

1. Create `deploy/adhive.service`
2. Create `install-systemd.sh`
3. Create backup scripts

---

## Implementation Order

### Phase 1: Configuration Unification (1 day)

1. Create `internal/config/config.go` with unified configuration
2. Update `cmd/server/main.go` to use new config
3. Update `.env.example` with new variables
4. Add backwards compatibility for old variables

### Phase 2: Docker Security (1 day)

1. Update Dockerfile with non-root user
2. Update docker-compose.yml with security options
3. Add health check
4. Test container builds

### Phase 3: Systemd Deployment (1 day)

1. Create systemd unit file
2. Create installation script
3. Create backup/restore scripts
4. Test on bare-metal deployment

### Phase 4: Documentation (0.5 day)

1. Update README.md with deployment instructions
2. Create deployment guide
3. Document environment variables

---

## File Structure

```
adhive/
├── cmd/
│   └── server/
│       └── main.go              # Updated to use unified config
├── internal/
│   └── config/
│       ├── config.go            # NEW: Unified configuration
│       └── storage.go           # Updated to use config
├── deploy/                      # NEW: Deployment files
│   ├── adhive.service           # Systemd unit file
│   ├── install-systemd.sh       # Installation script
│   ├── backup.sh                # Backup script
│   └── restore.sh               # Restore script
├── Dockerfile                   # Updated with security
├── docker-compose.yml           # Updated with security
├── .env.example                 # Updated with new variables
└── README.md                    # Updated deployment docs
```

---

## Testing Requirements

### Configuration Tests

```go
// internal/config/config_test.go

func TestDataDirUnification(t *testing.T) {
    tests := []struct {
        name     string
        envVars  map[string]string
        expected string
    }{
        {
            name:     "DATA_DIR takes precedence",
            envVars:  map[string]string{"DATA_DIR": "/data", "DB_PATH": "/db/test.db"},
            expected: "/data",
        },
        {
            name:     "Derive from STORAGE_DIR",
            envVars:  map[string]string{"STORAGE_DIR": "/storage"},
            expected: "/storage",
        },
        {
            name:     "Derive from DB_PATH",
            envVars:  map[string]string{"DB_PATH": "/var/lib/adhive/data/db"},
            expected: "/var/lib/adhive/data",
        },
        {
            name:     "Default to ./data",
            envVars:  map[string]string{},
            expected: "./data",
        },
    }
    // ... test implementation
}
```

### Docker Tests

```bash
# test-docker.sh

#!/bin/bash
set -e

# Build image
docker build -t adhive-test .

# Test 1: Non-root user
USER=$(docker run --rm adhive-test whoami)
if [ "$USER" != "adhive" ]; then
    echo "FAIL: Container should run as 'adhive' user, got '$USER'"
    exit 1
fi
echo "PASS: Running as non-root user"

# Test 2: Health check
docker run --rm -d --name adhive-test -p 8080:8080 adhive-test
sleep 10
curl -f http://localhost:8080/health || echo "FAIL: Health check failed"
docker stop adhive-test
echo "PASS: Health check working"

# Test 3: Data directory permissions
docker run --rm adhive-test ls -la /app/data
echo "PASS: Data directory accessible"
```

---

## Appendix A: Environment Variable Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `8080` | Server port |
| `DATA_DIR` | No | `./data` | Base data directory |
| `DB_PATH` | No | (derived) | Database file path |
| `STORAGE_DIR` | No | (derived) | Storage directory |
| `GO_ENV` | No | `development` | Environment mode |
| `LOG_LEVEL` | No | `info` | Logging level |
| `SESSION_SECRET` | Yes (prod) | - | Session signing secret |
| `HTTPS_ENABLED` | No | `false` | Enable HTTPS mode |
| `CORS_ALLOWED_ORIGINS` | No | (dev defaults) | CORS origins |
| `RATE_LIMIT_GLOBAL` | No | `100` | Global rate limit |
| `RATE_LIMIT_AUTH` | No | `5` | Auth rate limit |
| `USER_AGENT` | No | (auto) | Scraper user agent |
| `HUMAN_DOMAINS` | No | (empty) | Manual capture domains |

---

## Appendix B: Docker Security Best Practices

| Practice | Implementation |
|----------|----------------|
| Non-root user | `USER adhive` in Dockerfile |
| Minimal base image | Alpine Linux |
| Read-only filesystem | `ReadOnlyPaths` in systemd |
| No privilege escalation | `no-new-privileges:true` |
| Drop capabilities | `cap_drop: ALL` |
| Resource limits | `deploy.resources` |
| Health checks | `HEALTHCHECK` directive |
| Secrets management | Environment variables |

---

## Appendix C: Backup Strategy

| Data | Frequency | Retention | Method |
|------|-----------|-----------|--------|
| Database | Daily | 7 days | File copy |
| Archives | Daily | 30 days | rsync to backup |
| Config | On change | 5 versions | Version control |

---

*End of ADR-007*