.PHONY: all build test test-coverage lint proto clean docker run help

# Variables
BINARY_NAME := api-gateway
MAIN_PATH := ./cmd/server
PROTO_PATH := ./pkg/proto
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
DOCKER_IMAGE := pairdb/api-gateway
DOCKER_TAG := latest

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Build flags
LDFLAGS := -ldflags "-w -s -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

all: lint test build

## Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) $(MAIN_PATH)

## Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@echo "Coverage report:"
	$(GOCMD) tool cover -func=$(COVERAGE_FILE)

## Generate coverage HTML report
coverage-html: test-coverage
	@echo "Generating coverage HTML report..."
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

## Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

## Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## Generate protobuf files
proto:
	@echo "Generating protobuf files..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_PATH)/coordinator.proto

## Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)

## Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/docker/Dockerfile .

## Run the service locally
run: build
	@echo "Running $(BINARY_NAME)..."
	./bin/$(BINARY_NAME) --config config.yaml

## Run with hot reload (requires air)
dev:
	@echo "Running with hot reload..."
	air

## Verify all checks pass
verify: fmt lint test-coverage
	@echo "All checks passed!"

## Show help
help:
	@echo "API Gateway Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  all            - Run lint, test, and build"
	@echo "  build          - Build the binary"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  coverage-html  - Generate coverage HTML report"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  proto          - Generate protobuf files"
	@echo "  deps           - Download dependencies"
	@echo "  clean          - Clean build artifacts"
	@echo "  docker         - Build Docker image"
	@echo "  run            - Run the service locally"
	@echo "  dev            - Run with hot reload"
	@echo "  verify         - Run all checks"
	@echo "  help           - Show this help"

