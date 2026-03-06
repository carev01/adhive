# Build stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w -X main.version=latest" \
    -o ad-catalog ./cmd/server

# Final stage - minimal runtime image
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the binary
COPY --from=builder /app/ad-catalog /app/ad-catalog

# Create data directory
RUN mkdir -p /app/data /app/logs

WORKDIR /app

# Set environment variables
ENV PORT=8080
ENV DATA_DIR=/app/data
ENV LOG_LEVEL=info

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/ad-catalog"]
