#!/bin/bash

# Script to load Docker images into Minikube
# This is required because Minikube has its own Docker daemon

set -e

echo "====================================="
echo "Loading Docker Images into Minikube"
echo "====================================="

# Check if minikube is running
if ! minikube status > /dev/null 2>&1; then
    echo "Error: Minikube is not running. Please start it with 'minikube start'"
    exit 1
fi

# Function to load an image
load_image() {
    local image=$1
    echo ""
    echo "Loading image: $image"

    if docker images -q "$image" > /dev/null 2>&1; then
        minikube image load "$image"
        echo "✓ Successfully loaded $image"
    else
        echo "✗ Warning: Image $image not found locally. Please build it first."
        return 1
    fi
}

# Load all images
echo ""
echo "Step 1: Loading Storage Node image..."
load_image "pairdb/storage-node:latest" || echo "  → Run: cd ../storage-node && make docker-build"

echo ""
echo "Step 2: Loading Coordinator image..."
load_image "pairdb/coordinator:latest" || echo "  → Run: cd ../coordinator && make docker-build"

echo ""
echo "Step 3: Loading API Gateway image..."
load_image "pairdb/api-gateway:latest" || echo "  → Run: cd ../api-gateway && make docker-build"

echo ""
echo "====================================="
echo "Image loading complete!"
echo "====================================="
echo ""
echo "To verify images in Minikube, run:"
echo "  minikube ssh -- docker images | grep pairdb"
echo ""
