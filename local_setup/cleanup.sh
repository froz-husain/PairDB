#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Cleaning up PearDB Local Setup ===${NC}"

# Confirm deletion
read -p "This will delete all PearDB resources in the 'pairdb' namespace. Continue? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

# Delete namespace (this will delete all resources)
echo -e "${GREEN}Deleting namespace and all resources...${NC}"
kubectl delete namespace pairdb --ignore-not-found=true

# Wait for namespace to be deleted
if kubectl get namespace pairdb &> /dev/null; then
    echo "Waiting for namespace to be fully deleted..."
    kubectl wait --for=delete namespace/pairdb --timeout=60s || true
fi

echo -e "${GREEN}Cleanup complete!${NC}"

