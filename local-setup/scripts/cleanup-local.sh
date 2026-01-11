#!/bin/bash

# PairDB Local Cleanup Script
# This script removes PairDB from the local Minikube cluster

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="pairdb"

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check if namespace exists
if ! kubectl get namespace $NAMESPACE > /dev/null 2>&1; then
    print_warn "Namespace $NAMESPACE does not exist. Nothing to clean up."
    exit 0
fi

print_info "Deleting namespace: $NAMESPACE"
kubectl delete namespace $NAMESPACE

print_info "Waiting for namespace to be fully deleted..."
while kubectl get namespace $NAMESPACE > /dev/null 2>&1; do
    sleep 2
done

print_info "Cleanup complete!"
print_info ""
print_info "To redeploy, run: ./deploy-local.sh"
