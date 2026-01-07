.PHONY: all build clean proto run test docker-build docker-run

# Variables
BINARY_NAME=storage-node
PROTO_DIR=pkg/proto
GO_FILES=$(shell find . -name '*.go' -type f)
PROTO_FILES=$(shell find $(PROTO_DIR) -name '*.proto')

all: proto build

# Build the binary
build:
	@echo "Building storage node..."
	go build -o bin/$(BINARY_NAME) cmd/storage/main.go

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)

# Run the service
run: build
	@echo "Running storage node..."
	./bin/$(BINARY_NAME)

# Run all tests
test:
	@echo "Running all tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage-unit.out ./tests/unit/...

# Run integration tests only
test-integration:
	@echo "Running integration tests..."
	go test -v -race -timeout=5m ./tests/integration/...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -v -race ./...

# Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Check coverage threshold (80%)
test-coverage-check: test
	@echo "Checking coverage threshold..."
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ "$$(echo "$$coverage < 80" | bc)" -eq 1 ]; then \
		echo "Coverage $$coverage% is below 80% threshold"; \
		exit 1; \
	else \
		echo "Coverage $$coverage% meets 80% threshold"; \
	fi

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -benchtime=5s ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	find . -name '*.pb.go' -delete

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t pairdb-storage-node:latest .

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -p 50052:50052 -v $(PWD)/config.yaml:/app/config.yaml pairdb-storage-node:latest

# Install tools
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Help
help:
	@echo "Available targets:"
	@echo "  all            - Generate proto and build"
	@echo "  build          - Build the binary"
	@echo "  proto          - Generate protobuf code"
	@echo "  run            - Build and run the service"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  clean          - Clean build artifacts"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code"
	@echo "  deps           - Download dependencies"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run Docker container"
	@echo "  install-tools  - Install development tools"
	@echo "  help           - Show this help message"
