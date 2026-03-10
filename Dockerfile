# =============================================================================
# Stage 1: Frontend build
# =============================================================================
FROM node:24-alpine AS frontend-builder

WORKDIR /app/frontend

# Copy frontend dependencies
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# Copy frontend source
COPY frontend/ ./

# Build the frontend
RUN npm run build

# =============================================================================
# Stage 2: Go build
# =============================================================================
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/

# Build arguments for version info
ARG VERSION=latest
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the main server binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w \
        -X main.version=${VERSION} \
        -X main.commit=${COMMIT} \
        -X main.buildDate=${BUILD_DATE}" \
    -o ad-catalog ./cmd/server

# Build the import-shiori command
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w" \
    -o import-shiori ./cmd/import-shiori

# =============================================================================
# Stage 3: Playwright browser installation
# =============================================================================
# Use Debian-slim for Playwright (glibc required for bundled Chromium)
FROM node:24-slim AS playwright-builder

WORKDIR /app

# Install Playwright system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Playwright dependencies for Chromium
    libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
    libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 \
    libxrandr2 libgbm1 libasound2 libpango-1.0-0 libcairo2 \
    && rm -rf /var/lib/apt/lists/*

# Copy package files for Playwright
COPY package.json package-lock.json* ./
COPY playwright-scraper.js ./

# Install Node.js dependencies (Playwright + adblocker + autoconsent)
RUN npm ci --omit=dev

# Install Playwright browsers (Chromium only, native glibc support)
RUN npx playwright install chromium

# =============================================================================
# Stage 4: Final runtime image
# =============================================================================
# Use Debian-slim for Playwright compatibility (glibc-based)
FROM node:24-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    dumb-init \
    # Playwright dependencies for Chromium
    libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
    libdrm2 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 \
    libxrandr2 libgbm1 libasound2 libpango-1.0-0 libcairo2 \
    # Fonts for web rendering
    fonts-noto-cjk fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user and group
RUN groupadd -g 1001 adhive && \
    useradd -u 1001 -g adhive -m -s /bin/bash adhive

# Create required directories with proper ownership
RUN mkdir -p /app/static /app/data /app/logs /app/backups \
    /home/adhive/.cache \
    && chown -R adhive:adhive /app /home/adhive

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/ad-catalog /app/ad-catalog
COPY --from=builder /app/import-shiori /app/import-shiori
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy frontend static files to /app/static
COPY --from=frontend-builder --chown=adhive:adhive /app/frontend/build /app/static

# Copy Playwright scraper and node_modules
COPY --from=playwright-builder --chown=adhive:adhive /app/node_modules /app/node_modules
COPY --from=playwright-builder --chown=adhive:adhive /app/playwright-scraper.js /app/playwright-scraper.js
COPY --from=playwright-builder --chown=adhive:adhive /app/package.json /app/package.json

# Switch to non-root user
USER adhive

# Environment defaults
ENV PORT=8080 \
    DATA_DIR=/app/data \
    GO_ENV=production \
    LOG_LEVEL=info \
    HOME=/app \
    # Chromium flags for containerized environment
    CHROMIUM_FLAGS="--no-sandbox --disable-gpu --disable-dev-shm-usage --disable-setuid-sandbox"

# Expose port
EXPOSE 8080

# Health check - verify the server is responding
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--"]

# Run the server (supports subcommand for import-shiori)
# Default: run server. Use: docker run adhive import-shiori --help
CMD ["sh", "-c", "exec /app/ad-catalog"]