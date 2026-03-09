# AdHive Deployment Guide

This guide covers all deployment methods for AdHive, from development to production.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start (Docker Compose)](#quick-start-docker-compose)
3. [Production Deployment (Docker Compose)](#production-deployment-docker-compose)
4. [Bare Metal Deployment (Systemd)](#bare-metal-deployment-systemd)
5. [Kubernetes Deployment](#kubernetes-deployment)
6. [Development Environment](#development-environment)
7. [Configuration Reference](#configuration-reference)
8. [Backup and Restore](#backup-and-restore)
9. [Production Checklist](#production-checklist)
10. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### For Docker Deployment
- Docker 20.10+ 
- Docker Compose 2.0+
- 512MB RAM minimum, 1GB recommended
- 1GB disk space minimum

### For Bare Metal Deployment
- Linux server (Ubuntu 22.04+ or similar)
- Go 1.22+ (for building from source)
- Node.js 20+ (for building frontend)
- Systemd

### Ports
- **8080**: HTTP API and frontend (default)
- **5173**: Frontend dev server (development only)

---

## Quick Start (Docker Compose)

The fastest way to get AdHive running.

### 1. Clone and Configure

```bash
# Clone the repository
git clone https://github.com/your-org/adhive.git
cd adhive

# Create environment file
cp .env.example .env

# Edit the environment file
nano .env
```

### 2. Generate Session Secret

```bash
# Generate a secure session secret
openssl rand -hex 32

# Add it to your .env file
echo "SESSION_SECRET=your-generated-secret-here" >> .env
```

### 3. Start the Application

```bash
# Build and start
docker compose up -d

# Check logs
docker compose logs -f app
```

### 4. Access the Application

Open http://localhost:8080 in your browser.

### 5. Stop the Application

```bash
docker compose down
```

---

## Production Deployment (Docker Compose)

### 1. Prepare the Server

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Logout and login again for group changes
```

### 2. Clone and Configure

```bash
# Clone to a permanent location
git clone https://github.com/your-org/adhive.git /opt/adhive
cd /opt/adhive

# Create environment file
cp .env.example .env
```

### 3. Configure Production Settings

Edit `.env` with production values:

```bash
# REQUIRED
SESSION_SECRET=<generate-with-openssl-rand-hex-32>
HTTPS_ENABLED=true
GO_ENV=production

# Security
CORS_ALLOWED_ORIGINS=https://yourdomain.com

# Performance
RATE_LIMIT_GLOBAL=100
RATE_LIMIT_AUTH=5

# Logging
LOG_LEVEL=info
```

### 4. Set Up Reverse Proxy (Recommended)

Use Nginx or Caddy as a reverse proxy with HTTPS:

#### Option A: Nginx + Let's Encrypt

```nginx
# /etc/nginx/sites-available/adhive
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    # Security headers
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Content-Security-Policy "default-src 'self'" always;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

#### Option B: Caddy (Automatic HTTPS)

```caddyfile
# Caddyfile
yourdomain.com {
    reverse_proxy localhost:8080
    
    # Security headers are automatic with Caddy
}
```

### 5. Start Production Stack

```bash
# Build and start in production mode
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# Or with just the main compose file
docker compose up -d
```

### 6. Enable Automatic Updates (Optional)

```bash
# Install watchtower for automatic container updates
docker run -d \
  --name watchtower \
  -v /var/run/docker.sock:/var/run/docker.sock \
  containrrr/watchtower \
  --interval 43200 \
  adhive-app
```

---

## Bare Metal Deployment (Systemd)

For servers without Docker or when you need direct system integration.

### 1. Build from Source

```bash
# Install Go (if not already installed)
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Install Node.js (for frontend build)
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# Clone and build
git clone https://github.com/your-org/adhive.git /opt/adhive-src
cd /opt/adhive-src

# Build frontend
cd frontend
npm ci
npm run build
cd ..

# Build backend
make build

# Binary will be at ./bin/ad-catalog
```

### 2. Run Installation Script

```bash
# Run as root
sudo ./deploy/install-systemd.sh
```

This script will:
- Create `adhive` user and group
- Create directories (`/opt/adhive`, `/var/lib/adhive`, `/etc/adhive`)
- Copy the binary to `/opt/adhive/bin/`
- Install the systemd service
- Create configuration file at `/etc/adhive/config.env`

### 3. Configure

```bash
# Edit configuration
sudo nano /etc/adhive/config.env

# Generate session secret
openssl rand -hex 32
# Add to SESSION_SECRET=
```

### 4. Start the Service

```bash
# Enable and start
sudo systemctl enable adhive
sudo systemctl start adhive

# Check status
sudo systemctl status adhive

# View logs
sudo journalctl -u adhive -f
```

### 5. Set Up Reverse Proxy

Same as Docker deployment - use Nginx or Caddy as reverse proxy.

---

## Kubernetes Deployment

For cloud-native deployments with Kubernetes.

### 1. Create Namespace and Secrets

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: adhive
---
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: adhive-secrets
  namespace: adhive
type: Opaque
stringData:
  SESSION_SECRET: "your-secure-secret-here"
```

### 2. Create ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: adhive-config
  namespace: adhive
data:
  PORT: "8080"
  GO_ENV: "production"
  LOG_LEVEL: "info"
  DATA_DIR: "/app/data"
  HTTPS_ENABLED: "false"
  RATE_LIMIT_GLOBAL: "100"
  RATE_LIMIT_AUTH: "5"
```

### 3. Create Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: adhive
  namespace: adhive
spec:
  replicas: 1
  selector:
    matchLabels:
      app: adhive
  template:
    metadata:
      labels:
        app: adhive
    spec:
      containers:
      - name: adhive
        image: adhive:latest
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8080
        envFrom:
        - configMapRef:
            name: adhive-config
        - secretRef:
            name: adhive-secrets
        volumeMounts:
        - name: data
          mountPath: /app/data
        resources:
          limits:
            cpu: "1"
            memory: "512Mi"
          requests:
            cpu: "250m"
            memory: "128Mi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: adhive-data
```

### 4. Create Service and Ingress

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: adhive
  namespace: adhive
spec:
  selector:
    app: adhive
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: adhive
  namespace: adhive
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - yourdomain.com
    secretName: adhive-tls
  rules:
  - host: yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: adhive
            port:
              number: 80
```

### 5. Deploy

```bash
# Apply all manifests
kubectl apply -f namespace.yaml
kubectl apply -f secret.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
```

---

## Development Environment

### Prerequisites

- Go 1.22+
- Node.js 20+
- Make (optional)

### 1. Clone and Setup

```bash
git clone https://github.com/your-org/adhive.git
cd adhive
```

### 2. Backend Development

```bash
# Install Go dependencies
go mod download

# Run in development mode
go run ./cmd/server

# Or use Make
make dev
```

The API will be available at http://localhost:8080

### 3. Frontend Development

```bash
# Navigate to frontend
cd frontend

# Install dependencies
npm install

# Run development server
npm run dev
```

The frontend dev server will be at http://localhost:5173

### 4. Full Stack Development

```bash
# Terminal 1: Backend
go run ./cmd/server

# Terminal 2: Frontend
cd frontend && npm run dev

# Terminal 3: Docker services (if needed)
docker compose up -d
```

### 5. Environment Variables (Development)

Create `.env` in the project root:

```bash
# Development defaults
PORT=8080
DATA_DIR=./data
GO_ENV=development
LOG_LEVEL=debug
SESSION_SECRET=dev-secret-not-for-production

# Frontend
VITE_API_URL=http://localhost:8080
```

---

## Configuration Reference

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SESSION_SECRET` | **Yes** (prod) | - | Secret for session cookies. Generate with `openssl rand -hex 32` |
| `PORT` | No | `8080` | Server port |
| `DATA_DIR` | No | `./data` | Directory for database, archives, thumbnails |
| `GO_ENV` | No | `development` | Environment: `development` or `production` |
| `LOG_LEVEL` | No | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| `HTTPS_ENABLED` | No | `false` | Set to `true` if behind HTTPS proxy |
| `CORS_ALLOWED_ORIGINS` | No | - | Comma-separated list of allowed origins |
| `RATE_LIMIT_GLOBAL` | No | `100` | Global requests per minute |
| `RATE_LIMIT_AUTH` | No | `5` | Auth endpoint requests per minute |

### Directory Structure

```
data/
├── ad-catalog.db      # SQLite database
├── ad-catalog.db-wal  # Write-ahead log (if using WAL mode)
├── ad-catalog.db-shm  # Shared memory file
├── archives/          # Captured page archives
├── thumbnails/        # Generated thumbnails
└── logs/              # Application logs (optional)
```

---

## Backup and Restore

### Docker Compose Backup

```bash
# Using the provided backup script
DATA_DIR=./data BACKUP_DIR=./backups ./deploy/backup.sh

# Manual backup
docker compose exec app tar -czf /tmp/backup.tar.gz /app/data
docker compose cp app:/tmp/backup.tar.gz ./backup-$(date +%Y%m%d).tar.gz
```

### Docker Compose Restore

```bash
# Stop the service
docker compose down

# Restore data
tar -xzf backup-20240101.tar.gz

# Start the service
docker compose up -d
```

### Systemd Backup

```bash
# Using the provided script
sudo DATA_DIR=/var/lib/adhive/data BACKUP_DIR=/var/lib/adhive/backups \
    ./deploy/backup.sh

# Manual backup
sudo systemctl stop adhive
sudo tar -czf /var/lib/adhive/backups/adhive-$(date +%Y%m%d).tar.gz \
    -C /var/lib/adhive data
sudo systemctl start adhive
```

### Systemd Restore

```bash
# Using the provided script
sudo ./deploy/restore.sh /var/lib/adhive/backups/adhive-backup.tar.gz

# Manual restore
sudo systemctl stop adhive
sudo rm -rf /var/lib/adhive/data
sudo tar -xzf /var/lib/adhive/backups/adhive-backup.tar.gz -C /var/lib/adhive
sudo chown -R adhive:adhive /var/lib/adhive/data
sudo systemctl start adhive
```

### Automated Backups (Cron)

```bash
# Add to crontab
sudo crontab -e

# Daily backup at 2 AM
0 2 * * * DATA_DIR=/var/lib/adhive/data BACKUP_DIR=/var/lib/adhive/backups /opt/adhive/deploy/backup.sh >> /var/log/adhive-backup.log 2>&1
```

---

## Production Checklist

### Security

- [ ] **Session Secret**: Generate with `openssl rand -hex 32`
- [ ] **HTTPS**: Enabled via reverse proxy (Nginx, Caddy, Traefik)
- [ ] **CORS**: Configure `CORS_ALLOWED_ORIGINS` for your domains
- [ ] **Rate Limiting**: Adjust `RATE_LIMIT_GLOBAL` and `RATE_LIMIT_AUTH` as needed
- [ ] **Firewall**: Block external access to port 8080, only allow via reverse proxy
- [ ] **Updates**: Keep system and containers updated

### Configuration

- [ ] **GO_ENV=production**: Enable production mode
- [ ] **LOG_LEVEL=info**: Use `debug` only for troubleshooting
- [ ] **HTTPS_ENABLED=true**: When behind HTTPS proxy
- [ ] **Data Directory**: Use persistent volume/directory

### Monitoring

- [ ] **Health Check**: Configure `/health` endpoint monitoring
- [ ] **Logs**: Set up log aggregation (journalctl, Docker logs, or external)
- [ ] **Backups**: Configure automated daily backups
- [ ] **Alerts**: Set up alerts for service down

### Performance

- [ ] **Resource Limits**: Configure appropriate CPU/memory limits
- [ ] **Database**: Monitor database size and consider WAL mode
- [ ] **Archives**: Monitor archive storage usage

### High Availability (Optional)

- [ ] **Load Balancer**: Multiple instances behind load balancer
- [ ] **Shared Storage**: Use shared storage for data directory
- [ ] **Database**: Consider external SQLite replication or PostgreSQL migration

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker compose logs app

# Common issues:
# 1. Missing SESSION_SECRET
# 2. Permission issues on data directory
# 3. Port already in use
```

### Permission Denied Errors

```bash
# Docker: Fix data directory permissions
sudo chown -R 1000:1000 ./data

# Systemd: Fix ownership
sudo chown -R adhive:adhive /var/lib/adhive/data
```

### Database Locked Errors

```bash
# Check if WAL mode is enabled
sqlite3 data/ad-catalog.db "PRAGMA journal_mode;"

# Enable WAL mode (recommended)
sqlite3 data/ad-catalog.db "PRAGMA journal_mode=WAL;"
```

### Health Check Failing

```bash
# Test health endpoint manually
curl http://localhost:8080/health

# Check if service is listening
netstat -tlnp | grep 8080

# Docker: check container health
docker compose ps
docker inspect adhive-app | jq '.[0].State.Health'
```

### Memory Issues

```bash
# Check memory usage
docker stats adhive-app

# Increase memory limit in docker-compose.yml
deploy:
  resources:
    limits:
      memory: 1G
```

### Slow Performance

1. **Enable WAL mode** for SQLite (30-50% improvement)
2. **Check indexes** are created
3. **Monitor archive size** - large archives slow down
4. **Increase rate limits** if legitimate requests are blocked

---

## Support

- **Documentation**: [docs.adhive.ai](https://docs.adhive.ai)
- **Issues**: [GitHub Issues](https://github.com/your-org/adhive/issues)
- **Discussions**: [GitHub Discussions](https://github.com/your-org/adhive/discussions)