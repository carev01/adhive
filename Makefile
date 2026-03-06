# Makefile for Ad Catalog

.PHONY: build run test migrate clean docker-build docker-run lint fmt

# Build variables
BINARY_NAME=ad-catalog
PORT=8080
GO=go
GOFLAGS=-v -ldflags="-s -w"

# Default target
all: build

## Build the application
build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/server

## Run the application
run: build
	./bin/$(BINARY_NAME)

## Run in development mode (hot reload)
dev:
	air -c .air.conf

## Run tests
test:
	$(GO) test -v -race -coverprofile=coverage.out ./...

## Run tests with verbose output
test-verbose:
	$(GO) test -v ./...

## Run migrations
migrate-up:
	$(GO) run ./cmd/migrate -action up

migrate-down:
	$(GO) run ./cmd/migrate -action down

migrate-status:
	$(GO) run ./cmd/migrate -action status

migrate-create:
	$(GO) run ./cmd/migrate -action create -name "$$NAME"

## Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

## Run linter
lint:
	golangci-lint run ./...

## Format code
fmt:
	$(GO) fmt ./...
	gofmt -s -w .

## Docker: Build the image
docker-build:
	docker build -t $(BINARY_NAME):latest .

## Docker: Run the container
docker-run:
	docker run -it --rm -p $(PORT):$(PORT) -v $(PWD)/data:/app/data $(BINARY_NAME):latest

## Docker: Build and run with docker-compose
docker-up:
	docker-compose up --build

## Docker: Stop services
docker-down:
	docker-compose down

## Install development dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

## Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  run           - Build and run the application"
	@echo "  dev           - Run with hot reload"
	@echo "  test          - Run tests with coverage"
	@echo "  migrate       - Run database migrations"
	@echo "  clean         - Clean build artifacts"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  docker-up     - Start with docker-compose"
	@echo "  docker-down   - Stop docker-compose services"
