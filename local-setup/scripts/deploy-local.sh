#!/bin/bash

# PairDB Local Deployment Script
# This script deploys PairDB to a local Minikube cluster

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="pairdb"
# Use timestamp-based tags to avoid Minikube image caching issues
IMAGE_TAG="v$(date +%Y%m%d-%H%M%S)"
COORDINATOR_IMAGE="pairdb/coordinator:${IMAGE_TAG}"
STORAGE_NODE_IMAGE="pairdb/storage-node:${IMAGE_TAG}"

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if minikube is running
print_info "Checking Minikube status..."
if ! minikube status > /dev/null 2>&1; then
    print_error "Minikube is not running. Please start it with 'minikube start'"
    exit 1
fi

print_info "Minikube is running"

# Create namespace if it doesn't exist
print_info "Creating namespace: $NAMESPACE"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Build coordinator image
print_info "Building coordinator image: $COORDINATOR_IMAGE"
cd ../../coordinator
docker build -t $COORDINATOR_IMAGE -f deployments/docker/Dockerfile .
if [ $? -ne 0 ]; then
    print_error "Failed to build coordinator image"
    exit 1
fi
print_info "Coordinator image built successfully"

# Build storage-node image
print_info "Building storage-node image: $STORAGE_NODE_IMAGE"
cd ../storage-node
docker build -t $STORAGE_NODE_IMAGE -f Dockerfile .
if [ $? -ne 0 ]; then
    print_error "Failed to build storage-node image"
    exit 1
fi
print_info "Storage-node image built successfully"

# Load images into minikube
print_info "Loading images into Minikube..."
print_info "Loading coordinator image..."
minikube image load $COORDINATOR_IMAGE
if [ $? -ne 0 ]; then
    print_error "Failed to load coordinator image into Minikube"
    exit 1
fi

print_info "Loading storage-node image..."
minikube image load $STORAGE_NODE_IMAGE
if [ $? -ne 0 ]; then
    print_error "Failed to load storage-node image into Minikube"
    exit 1
fi
print_info "Images loaded successfully"

# Verify images are loaded
print_info "Verifying images in Minikube..."
if ! minikube image ls | grep -q "$COORDINATOR_IMAGE"; then
    print_error "Coordinator image not found in Minikube"
    exit 1
fi
if ! minikube image ls | grep -q "$STORAGE_NODE_IMAGE"; then
    print_error "Storage-node image not found in Minikube"
    exit 1
fi
print_info "Images verified successfully"

# Deploy infrastructure (PostgreSQL, Redis)
print_info "Deploying infrastructure components..."
cd ../local-setup/k8s

kubectl apply -f namespace.yaml
kubectl apply -f postgres/
kubectl apply -f redis/

# Wait for infrastructure to be ready
print_info "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres -n $NAMESPACE --timeout=120s

print_info "Waiting for Redis to be ready..."
kubectl wait --for=condition=ready pod -l app=redis -n $NAMESPACE --timeout=120s

# Deploy coordinator
print_info "Deploying coordinator..."
kubectl apply -f coordinator/

# Wait a bit for coordinator deployment to be created
sleep 5

# Scale down coordinator to force image reload
print_info "Reloading coordinator with new image..."
kubectl scale deployment/coordinator -n $NAMESPACE --replicas=0
sleep 5
kubectl set image deployment/coordinator -n $NAMESPACE coordinator=$COORDINATOR_IMAGE
kubectl scale deployment/coordinator -n $NAMESPACE --replicas=2

# Deploy storage-node
print_info "Deploying storage-node..."
kubectl apply -f storage-node/

# Update StatefulSet to use the new image tag
print_info "Updating storage-node to use image: $STORAGE_NODE_IMAGE"
kubectl set image statefulset/pairdb-storage-node -n $NAMESPACE storage-node=$STORAGE_NODE_IMAGE

# Wait for services to start
print_info "Waiting for services to start (this may take a minute)..."
sleep 30

# Check pod status
print_info "Current pod status:"
kubectl get pods -n $NAMESPACE

# Check if coordinator is running
COORDINATOR_RUNNING=$(kubectl get pods -n $NAMESPACE -l app=coordinator --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$COORDINATOR_RUNNING" -ge "1" ]; then
    print_info "✓ Coordinator is running ($COORDINATOR_RUNNING pods)"
else
    print_warn "⚠ Coordinator is not running yet. Check logs with: kubectl logs -n $NAMESPACE -l app=coordinator"
fi

# Check if storage-node is running
STORAGE_RUNNING=$(kubectl get pods -n $NAMESPACE -l app=pairdb-storage-node --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$STORAGE_RUNNING" -ge "1" ]; then
    print_info "✓ Storage-node is running ($STORAGE_RUNNING pods)"
else
    print_warn "⚠ Storage-node is not running yet. Check logs with: kubectl logs -n $NAMESPACE -l app=pairdb-storage-node"
fi

print_info ""
print_info "================================================"
print_info "Deployment Complete!"
print_info "================================================"
print_info ""
print_info "Images deployed:"
print_info "  Coordinator:   $COORDINATOR_IMAGE"
print_info "  Storage-node:  $STORAGE_NODE_IMAGE"
print_info ""
print_info "Useful commands:"
print_info "  View all pods:           kubectl get pods -n $NAMESPACE"
print_info "  View coordinator logs:   kubectl logs -n $NAMESPACE -l app=coordinator -f"
print_info "  View storage-node logs:  kubectl logs -n $NAMESPACE -l app=pairdb-storage-node -f"
print_info "  Delete deployment:       kubectl delete namespace $NAMESPACE"
print_info ""
print_info "Note: Health probes are temporarily disabled for storage-node."
print_info "      HTTP health server implementation needs debugging."
print_info ""
