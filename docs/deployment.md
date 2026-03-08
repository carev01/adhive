# Deployment Guide

This guide covers deploying AdHive in various environments—from local development to production self-hosting.

---

## Table of Contents

1. [Local Development Setup](#local-development-setup)
2. [Docker Deployment](#docker-deployment)
3. [Production Considerations](#production-considerations)
4. [Environment Variables Reference](#environment-variables-reference)
5. [Troubleshooting](#troubleshooting)

---

## Local Development Setup

### Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.22+ | Backend runtime |
| Node.js | 20+ | Frontend build |
| SQLite | 3.x | Built-in, no install needed |
| Docker | 24+ | Optional, for containerized dev |

### Quick Start (Native)

```bash
# 1. Clone and navigate to project
cd ~/OpenClaw/Projects/adhive

# 2. Copy environment template
cp .env.example .env

# 3. Install Go dependencies
make deps
# or: go mod download

# 4. Build the application
make build
# or: go build -o ad-catalog ./cmd/server

# 5. Run the server
make run
# or: ./ad-catalog
```

The API server starts on `http://localhost:8080`.

### Frontend Development

```bash
# In a separate terminal
cd frontend
npm install
npm run dev
```

The frontend runs on `http://localhost:5173` and proxies API requests to the backend.

### Using Docker Compose (Local Dev)

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

This starts:
- **app** — Go API server on port 8080
- **frontend** — SvelteKit dev server on port 5173

---

## Docker Deployment

### Building the Image

```bash
# Build using Makefile
make docker-build

# Or directly with docker
docker build -t adhive:latest .
```

### Running the Container

```bash
# Basic run
docker run -d \
  --name adhive \
  -p 8080:8080 \
  -v adhive-data:/app/data \
  adhive:latest
```

### With Docker Compose (Recommended)

```bash
# Production-style deployment
docker-compose up -d

# Scale if needed (currently single-instance)
docker-compose up -d --scale app=2
```

### Persistent Storage

The `docker-compose.yml` defines a named volume `ad-catalog-data` for persistent storage:

```yaml
volumes:
  - ad-catalog-data:/app/data
```

To backup data:
```bash
docker run --rm -v adhive_ad-catalog-data:/data -v $(pwd):/backup alpine tar czf /backup/adhive-data.tar.gz /data
```

---

## Production Considerations

### 1. Database

- **SQLite** works well for small-to-medium workloads
- Enable WAL mode for better concurrency (default in Go's sqlite driver)
- Set up **litestream** for continuous backup (see docker-compose.yml commented section)

### 2. Security

```bash
# Generate a secure session secret
openssl rand -base64 32
```

Add to your `.env`:
```
SESSION_SECRET=your-generated-secret-here
```

### 3. Reverse Proxy (Recommended)

Deploy behind nginx or Traefik:

**nginx example:**
```nginx
server {
    listen 80;
    server_name adhive.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 4. TLS/SSL

Use Let's Encrypt with Certbot or your reverse proxy's built-in TLS support.

### 5. Resource Limits

In production, constrain container resources:

```yaml
services:
  app:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
```

### 6. Health Checks

The container includes a health check:
```bash
curl http://localhost:8080/health
```

### 7. Logging

- Set `LOG_LEVEL=debug` for verbose logs
- In production, aggregate logs with a logging sidecar or use `docker logs -f adhive-app`

### 8. Backup Strategy

For production SQLite:
1. Use **litestream** for continuous replication (see docker-compose.yml)
2. Or schedule periodic dumps:
```bash
docker exec adhive-app sqlite3 /app/data/ad-catalog.db ".backup /backup/db-$(date +%Y%m%d).db"
```

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | 8080 | HTTP server port |
| `DB_PATH` | No | ad-catalog.db | SQLite database file path |
| `STORAGE_DIR` | No | ./data | Base directory for file storage (archives, thumbnails) |
| `USER_AGENT` | No | auto-detect | Custom Chrome User-Agent for scraping |
| `HUMAN_DOMAINS` | No | — | Comma-separated domains requiring headful mode (CAPTCHA handling) |
| `SESSION_SECRET` | No | — | Secret for JWT signing. **Set in production!** |
| `LOG_LEVEL` | No | info | Log level: debug, info, warn, error |

### Generating a Secure Session Secret

```bash
# Linux/macOS
openssl rand -base64 32

# Or with Python
python3 -c "import secrets; print(secrets.token_urlsafe(32))"
```

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs adhive-app

# Verify volume permissions
docker exec adhive-app ls -la /app/data
```

### Database Locked Errors

- Multiple app instances sharing the same SQLite file
- Solution: Use a single instance, or switch to PostgreSQL (requires code changes)

### Port Already in Use

```bash
# Find what's using port 8080
lsof -i :8080

# Kill or change PORT in .env
```

### Health Check Fails

```bash
# Test manually
curl http://localhost:8080/health

# Check container status
docker inspect adhive-app | grep -A 10 Health
```

### Out of Memory

The Go binary is compiled with CGO disabled and runs in a `scratch` image—very lightweight. If you hit memory limits:

```bash
# Monitor usage
docker stats adhive-app

# Increase memory limit in docker-compose.yml or container runtime
```

### Frontend Can't Connect to API

Ensure both services are on the same Docker network:
```bash
# In docker-compose, frontend uses VITE_API_URL=http://app:8080
# Verify network
docker network inspect adhive-network
```

### Data Loss After Container Restart

- Ensure the volume is correctly defined and mounted
- Check: `docker volume inspect adhive_ad-catalog-data`

### Need to Reset Development Environment

```bash
# Stop and remove containers + volumes
docker-compose down -v

# Remove local database
rm -f data/*.db

# Rebuild
docker-compose build --no-cache
docker-compose up -d
```

---

## Next Steps

- See [API Documentation](./api.md) for endpoint details
- See [Architecture Overview](./architecture.md) for system design
