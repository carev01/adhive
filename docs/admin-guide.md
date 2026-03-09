# AdHive Admin Guide

A comprehensive guide for system administrators deploying and maintaining AdHive.

## Table of Contents

1. [Installation](#installation)
   - [Docker](#docker)
   - [Systemd (Bare Metal)](#systemd-bare-metal)
2. [Configuration](#configuration)
   - [Environment Variables](#environment-variables)
   - [Data Directory](#data-directory)
3. [Security](#security)
   - [Hardening](#hardening)
   - [HTTPS](#https)
   - [Rate Limiting](#rate-limiting)
4. [Backup and Restore](#backup-and-restore)
   - [Backup](#backup)
   - [Restore](#restore)
   - [Automated Backups](#automated-backups)
5. [Monitoring](#monitoring)
   - [Health Checks](#health-checks)
   - [Logs](#logs)
6. [Troubleshooting](#troubleshooting)
   - [Common Issues](#common-issues)
   - [Debug Mode](#debug-mode)
7. [Performance Tuning](#performance-tuning)

---

## Installation

### Docker

The recommended way to run AdHive in production.

#### Prerequisites

- Docker 20.10+
- Docker Compose 2.0+ (optional)
- 512MB RAM minimum
- 1GB disk space (plus data)

#### Quick Start

```bash
# Clone and navigate to project
git clone https://github.com/adhive/adhive.git
cd adhive

# Create .env file
cp .env.example .env
# Edit .env and set SESSION_SECRET

# Start the container
docker build -t adhive .
docker run -d -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e SESSION_SECRET=your-secret-here \
  adhive
```

#### Using Docker Compose

```bash
# Start all services
docker compose up -d

# View logs
docker compose logs -f

# Stop services
docker compose down
```

#### Production Docker Settings

Create a `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  app:
    build: .
    restart: unless-stopped
    user: "1000:1000"
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - DATA_DIR=/app/data
      - GO_ENV=production
      - LOG_LEVEL=info
      - SESSION_SECRET=${SESSION_SECRET:?Required}
      - HTTPS_ENABLED=false
      - CORS_ALLOWED_ORIGINS=https://yourdomain.com
      - RATE_LIMIT_GLOBAL=100
      - RATE_LIMIT_AUTH=5
    volumes:
      - ./data:/app/data
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
```

### Systemd (Bare Metal)

For running AdHive directly on a Linux server without Docker.

#### Prerequisites

- Go 1.22+
- Linux server (Ubuntu 20.04+, Debian 11+, or similar)
- SQLite support in OS

#### Installation

```bash
# Clone the repository
git clone https://github.com/adhive/adhive.git
cd adhive

# Run the installation script
sudo ./deploy/install-systemd.sh

# Edit configuration
sudo nano /etc/adhive/config.env
```

#### Service Management

```bash
# Start
sudo systemctl start adhive

# Stop
sudo systemctl stop adhive

# Restart
sudo systemctl restart adhive

# View status
sudo systemctl status adhive

# View logs
sudo journalctl -u adhive -f
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `8080` | Server port |
| `DATA_DIR` | No | `./data` | Base data directory |
| `GO_ENV` | No | `development` | Environment mode |
| `LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `SESSION_SECRET` | Yes (prod) | - | Secret for session cookies |
| `HTTPS_ENABLED` | No | `false` | Enable HTTPS mode |
| `CORS_ALLOWED_ORIGINS` | No | - | Comma-separated CORS origins |
| `RATE_LIMIT_GLOBAL` | No | `100` | Global rate limit (per minute) |
| `RATE_LIMIT_AUTH` | No | `5` | Auth endpoint rate limit |

### Data Directory

The `DATA_DIR` contains all persistent data:

```
DATA_DIR/
├── ad-catalog.db      # SQLite database
├── archives/          # Archived ads
├── thumbnails/       # Image thumbnails
├── logs/             # Application logs
└── backups/          # Backup files
```

**Important**: Always backup this directory before updates.

---

## Security

### Hardening

1. **Run as non-root**: The Docker container runs as user `adhive` (UID 1000)
2. **Drop capabilities**: Only `NET_BIND_SERVICE` is added
3. **No privilege escalation**: `no-new-privileges:true`
4. **Read-only filesystem**: Use with Docker `--read-only` flag for extra security
5. **Firewall**: Only expose port 8080 (or your configured port)

```bash
# Example firewall rules (UFW)
sudo ufw allow 8080/tcp
sudo ufw enable
```

### HTTPS

For production, enable HTTPS:

```yaml
# docker-compose.yml
environment:
  - HTTPS_ENABLED=true
  - CORS_ALLOWED_ORIGINS=https://yourdomain.com
```

You'll need to configure a reverse proxy (nginx, Caddy, or Traefik) for TLS termination.

### Rate Limiting

Rate limiting is built-in:

- **Global**: Default 100 requests/minute per IP
- **Auth**: Default 5 requests/minute for login/register endpoints

Adjust in environment:

```yaml
environment:
  - RATE_LIMIT_GLOBAL=100
  - RATE_LIMIT_AUTH=5
```

---

## Backup and Restore

### Backup

#### Docker

```bash
# Manual backup
docker exec adhive-app ./backup.sh

# Or from host
docker run --rm -v adhive-data:/app/data -v $(pwd):/backup adhive \
  tar czf /backup/adhive-backup-$(date +%Y%m%d).tar.gz -C /app/data .
```

#### Systemd

```bash
# Run backup script
sudo /opt/adhive/bin/backup.sh

# With custom retention
sudo /opt/adhive/bin/backup.sh --keep 30
```

### Restore

```bash
# Docker
docker run --rm -v adhive-data:/app/data -v $(pwd):/backup adhive \
  tar xzf /backup/adhive-backup-20240101.tar.gz -C /app/data

# Systemd
sudo /opt/adhive/bin/restore.sh /var/lib/adhive/backups/adhive-backup-20240101.tar.gz --stop
```

### Automated Backups

Add to crontab for daily backups:

```bash
# Edit crontab
sudo crontab -e

# Add line for daily backup at 2 AM
0 2 * * * /opt/adhive/bin/backup.sh
```

---

## Monitoring

### Health Checks

The service provides a `/health` endpoint:

```bash
# Check health
curl http://localhost:8080/health

# Docker health check
docker inspect --format='{{.State.Health.Status}}' adhive-app
```

### Logs

#### Docker

```bash
# View logs
docker logs adhive-app

# Follow logs
docker logs -f adhive-app

# With timestamps
docker logs -t adhive-app
```

#### Systemd

```bash
# View recent logs
sudo journalctl -u adhive -n 100

# Follow logs
sudo journalctl -u adhive -f

# Since specific time
sudo journalctl -u adhive --since "1 hour ago"
```

#### Log Levels

Set via `LOG_LEVEL` environment variable:
- `debug`: Most verbose
- `info`: Default
- `warn`: Warnings only
- `error`: Errors only

---

## Troubleshooting

### Common Issues

#### Container won't start

```bash
# Check logs
docker logs adhive-app

# Common causes:
# - Port already in use: Change PORT in .env
# - Permission denied: Check volume mounts
# - Missing SESSION_SECRET: Add to .env
```

#### Database locked errors

```bash
# Only one instance can write to SQLite
# If using multiple replicas, use PostgreSQL
# Or ensure only one instance is running
docker ps | grep adhive
```

#### Slow performance

```bash
# Check resource usage
docker stats adhive-app

# Increase memory limit in docker-compose.yml
deploy:
  resources:
    limits:
      memory: 1G
```

#### Can't connect to service

```bash
# Check if service is running
docker ps | grep adhive
# or
sudo systemctl status adhive

# Check port binding
ss -tlnp | grep 8080

# Check firewall
sudo ufw status
```

### Debug Mode

Enable debug logging:

```yaml
# docker-compose.yml
environment:
  - LOG_LEVEL=debug
```

Or for systemd, edit `/etc/adhive/config.env`:

```
LOG_LEVEL=debug
```

Then restart the service:

```bash
# Docker
docker compose restart

# Systemd
sudo systemctl restart adhive
```

---

## Performance Tuning

### Resource Limits

Adjust based on your workload:

```yaml
# docker-compose.yml
deploy:
  resources:
    limits:
      cpus: '2'        # Increase for high traffic
      memory: 1G      # Increase for large datasets
    reservations:
      cpus: '0.5'
      memory: 256M
```

### SQLite Optimization

For better SQLite performance:

```bash
# In your data directory
# SQLite auto-vacuum is enabled by default
# For manual maintenance:
sqlite3 ad-catalog.db "VACUUM;"
sqlite3 ad-catalog.db "ANALYZE;"
```

### Frontend Caching

The nginx configuration includes caching for static assets:

```nginx
# Cache static assets for 1 year
location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
    expires 1y;
    add_header Cache-Control "public, immutable";
}
```

---

## Support

- **GitHub Issues**: https://github.com/adhive/adhive/issues
- **Documentation**: https://docs.adhive.ai
- **Discussions**: https://github.com/adhive/adhive/discussions

---

*Last updated: 2026-03-09*
