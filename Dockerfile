# =============================================================================
# Stage 1: Frontend build
# =============================================================================
FROM node:20-alpine AS frontend-builder

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
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

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
# Stage 3: Final runtime image
# =============================================================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata dumb-init

# Create non-root user and group
RUN addgroup -g 1000 -S adhive && \
    adduser -u 1000 -S -G adhive -s /bin/sh adhive

# Create required directories with proper ownership
RUN mkdir -p /app/static /app/data /app/logs /app/backups && \
    chown -R adhive:adhive /app

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/ad-catalog /app/ad-catalog
COPY --from=builder /app/import-shiori /app/import-shiori
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy frontend static files to /app/static
COPY --from=frontend-builder --chown=adhive:adhive /app/frontend/.svelte-kit/output/client /app/static

# Switch to non-root user
USER adhive

# Environment defaults
ENV PORT=8080 \
    DATA_DIR=/app/data \
    GO_ENV=production \
    LOG_LEVEL=info

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
