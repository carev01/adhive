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
FROM node:24-alpine AS playwright-builder

WORKDIR /app

# Copy package files for Playwright
COPY package.json package-lock.json* ./
COPY playwright-scraper.js ./

# Install Node.js dependencies (Playwright + adblocker + autoconsent)
RUN npm ci --omit=dev

# Install Playwright browsers (Chromium only) - Alpine's system chromium
# We'll use Alpine's chromium package in the runtime image instead
RUN npx playwright install chromium

# =============================================================================
# Stage 4: Final runtime image
# =============================================================================
FROM node:24-alpine

# Install runtime dependencies for Chromium
# Alpine's chromium package pulls in most dependencies automatically
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    dumb-init \
    # Chromium and its dependencies
    chromium \
    # Additional fonts
    font-noto-cjk \
    font-noto-emoji \
    # Required for Playwright's bundled Chromium (glibc compatibility)
    libstdc++ \
    libgcc \
    # X11 libs that may not be pulled in automatically
    libx11 \
    libxcb \
    libxcomposite \
    libxdamage \
    libxrandr \
    libxfixes \
    libxkbcommon \
    mesa-gl \
    # Audio support
    alsa-lib \
    # DBus for browser communication
    dbus-libs

# Create non-root user and group
# Use UID 1001 to avoid conflict with base image's user
RUN addgroup -g 1001 -S adhive && \
    adduser -u 1001 -S -G adhive -s /bin/sh adhive

# Create required directories with proper ownership
# Playwright needs a cache directory and tmp for profile data
RUN mkdir -p /app/static /app/data /app/logs /app/backups \
    /home/adhive/.cache \
    /tmp/playwright \
    && chown -R adhive:adhive /app /home/adhive /tmp/playwright

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

# Playwright is installed but we'll use Alpine's system Chromium
# The system chromium is installed by the apk package in the runtime stage

# Switch to non-root user
USER adhive

# Environment defaults
ENV PORT=8080 \
    DATA_DIR=/app/data \
    GO_ENV=production \
    LOG_LEVEL=info \
    # Playwright configuration - use Alpine's system Chromium
    PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium-browser \
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