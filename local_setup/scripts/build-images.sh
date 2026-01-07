#!/bin/bash

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo -e "${GREEN}=== Building Docker Images for PearDB ===${NC}"

# Check if minikube is running
if ! minikube status &> /dev/null; then
    echo -e "${YELLOW}Minikube is not running. Starting Minikube...${NC}"
    minikube start --memory=4096 --cpus=2
fi

# Set docker environment to use minikube's docker
eval $(minikube docker-env)

# Build API Gateway
echo -e "${GREEN}[1/3] Building API Gateway image...${NC}"
cd "${ROOT_DIR}/api-gateway"
if [ -f "Makefile" ] && grep -q "docker" Makefile; then
    make docker || {
        echo "Building with docker build..."
        docker build -t pairdb/api-gateway:latest -f deployments/docker/Dockerfile .
    }
else
    docker build -t pairdb/api-gateway:latest -f deployments/docker/Dockerfile .
fi

# Build Coordinator
echo -e "${GREEN}[2/3] Building Coordinator image...${NC}"
cd "${ROOT_DIR}/coordinator"
if [ -f "Makefile" ] && grep -q "docker-build" Makefile; then
    make docker-build || {
        echo "Building with docker build..."
        docker build -t pairdb/coordinator:latest -f deployments/docker/Dockerfile .
    }
else
    docker build -t pairdb/coordinator:latest -f deployments/docker/Dockerfile .
fi

# Build Storage Node
echo -e "${GREEN}[3/3] Building Storage Node image...${NC}"
cd "${ROOT_DIR}/storage-node"
if [ -f "Makefile" ] && grep -q "docker-build" Makefile; then
    make docker-build || {
        echo "Building with docker build..."
        docker build -t pairdb/storage-node:latest -f Dockerfile .
    }
else
    docker build -t pairdb/storage-node:latest -f Dockerfile .
fi

echo ""
echo -e "${GREEN}=== Image Build Complete ===${NC}"
echo ""
echo "Images built:"
echo "  - pairdb/api-gateway:latest"
echo "  - pairdb/coordinator:latest"
echo "  - pairdb/storage-node:latest"
echo ""
echo "Images are now available in Minikube's Docker environment."

