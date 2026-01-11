.PHONY: all build clean test proto lint fmt docker-build docker-push k8s-deploy help

# Variables
SERVICE_NAME := coordinator
BINARY_NAME := coordinator
DOCKER_IMAGE := pairdb/coordinator
VERSION := $(shell git describe --tags --always --dirty)
BUILD_DIR := bin
PROTO_DIR := api
PROTO_OUT_DIR := pkg/proto

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')"

all: clean proto build test

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

proto: ## Generate proto files
	@echo "Generating proto files..."
	@mkdir -p $(PROTO_OUT_DIR)
	@export PATH="$$PATH:$$(go env GOPATH)/bin" && \
	protoc -I. --go_out=. --go_opt=module=github.com/devrev/pairdb/coordinator \
		--go-grpc_out=. --go-grpc_opt=module=github.com/devrev/pairdb/coordinator \
		$(PROTO_DIR)/*.proto
	@echo "Proto files generated successfully"

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/coordinator
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf $(PROTO_OUT_DIR)
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	$(GOTEST) -v -race -cover ./tests/unit/...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) -v -race -cover ./tests/integration/...

coverage: ## Generate coverage report
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run linter
	@echo "Running linter..."
	$(GOLINT) run ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	$(GOCMD) mod tidy

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) -f deployments/docker/Dockerfile .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest
	@echo "Docker image built: $(DOCKER_IMAGE):$(VERSION)"

docker-push: ## Push Docker image
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

docker-run: ## Run Docker container locally
	docker run -p 50051:50051 \
		-e DATABASE_HOST=localhost \
		-e REDIS_HOST=localhost \
		$(DOCKER_IMAGE):latest

k8s-deploy: ## Deploy to Kubernetes
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/k8s/

k8s-delete: ## Delete from Kubernetes
	@echo "Deleting from Kubernetes..."
	kubectl delete -f deployments/k8s/

run: ## Run the service locally
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run ./cmd/coordinator --config config.yaml

install: build ## Install the binary
	@echo "Installing $(BINARY_NAME)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOCMD) vet ./...

mod-update: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

generate: ## Run go generate
	@echo "Running go generate..."
	$(GOCMD) generate ./...

.DEFAULT_GOAL := help
