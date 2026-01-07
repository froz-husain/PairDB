#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="${SCRIPT_DIR}/k8s"

echo -e "${GREEN}=== PearDB Local Setup Deployment ===${NC}"

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl is not installed${NC}"
    exit 1
fi

# Check if minikube is running
if ! kubectl config current-context | grep -q minikube; then
    echo -e "${YELLOW}Warning: Not using minikube context. Make sure you're deploying to the correct cluster.${NC}"
    read -p "Continue? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Step 1: Create namespace
echo -e "${GREEN}[1/7] Creating namespace...${NC}"
kubectl apply -f "${K8S_DIR}/namespace.yaml"
kubectl wait --for=condition=Active namespace/pairdb --timeout=30s || true

# Step 2: Deploy PostgreSQL
echo -e "${GREEN}[2/7] Deploying PostgreSQL...${NC}"
kubectl apply -f "${K8S_DIR}/postgres/"
echo "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres -n pairdb --timeout=120s || {
    echo -e "${RED}PostgreSQL failed to start. Check logs with: kubectl logs -l app=postgres -n pairdb${NC}"
    exit 1
}

# Step 3: Deploy Redis
echo -e "${GREEN}[3/7] Deploying Redis...${NC}"
kubectl apply -f "${K8S_DIR}/redis/"
echo "Waiting for Redis to be ready..."
kubectl wait --for=condition=ready pod -l app=redis -n pairdb --timeout=120s || {
    echo -e "${RED}Redis failed to start. Check logs with: kubectl logs -l app=redis -n pairdb${NC}"
    exit 1
}

# Step 4: Initialize database
echo -e "${GREEN}[4/7] Initializing database...${NC}"
PG_POD=$(kubectl get pod -l app=postgres -n pairdb -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$PG_POD" ]; then
    echo -e "${RED}PostgreSQL pod not found${NC}"
    exit 1
fi

# Wait a bit more for PostgreSQL to be fully ready
sleep 5

# Check if migration already exists
if kubectl exec -n pairdb "$PG_POD" -- psql -U coordinator -d pairdb_metadata -c "\dt" 2>/dev/null | grep -q tenants; then
    echo -e "${YELLOW}Database already initialized, skipping migration${NC}"
else
    echo "Running database migration..."
    MIGRATION_FILE="${SCRIPT_DIR}/../coordinator/migrations/001_initial_schema.sql"
    if [ ! -f "$MIGRATION_FILE" ]; then
        echo -e "${YELLOW}Migration file not found at $MIGRATION_FILE, skipping...${NC}"
    else
        kubectl cp "$MIGRATION_FILE" "pairdb/$PG_POD:/tmp/001_initial_schema.sql"
        kubectl exec -n pairdb "$PG_POD" -- psql -U coordinator -d pairdb_metadata -f /tmp/001_initial_schema.sql || {
            echo -e "${YELLOW}Migration may have already been applied${NC}"
        }
    fi
fi

# Step 5: Deploy Storage Nodes
echo -e "${GREEN}[5/7] Deploying Storage Nodes...${NC}"
kubectl apply -f "${K8S_DIR}/storage-node/"
echo "Waiting for Storage Nodes to be ready..."
kubectl wait --for=condition=ready pod -l app=pairdb-storage-node -n pairdb --timeout=180s || {
    echo -e "${YELLOW}Storage nodes may still be starting. Check status with: kubectl get pods -n pairdb${NC}"
}

# Step 6: Deploy Coordinator
echo -e "${GREEN}[6/7] Deploying Coordinator...${NC}"
kubectl apply -f "${K8S_DIR}/coordinator/"
echo "Waiting for Coordinator to be ready..."
kubectl wait --for=condition=ready pod -l app=coordinator -n pairdb --timeout=120s || {
    echo -e "${YELLOW}Coordinator may still be starting. Check status with: kubectl get pods -n pairdb${NC}"
}

# Step 7: Deploy API Gateway
echo -e "${GREEN}[7/7] Deploying API Gateway...${NC}"
kubectl apply -f "${K8S_DIR}/api-gateway/"
echo "Waiting for API Gateway to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=api-gateway -n pairdb --timeout=120s || {
    echo -e "${YELLOW}API Gateway may still be starting. Check status with: kubectl get pods -n pairdb${NC}"
}

# Summary
echo ""
echo -e "${GREEN}=== Deployment Complete ===${NC}"
echo ""
echo "Deployment Status:"
kubectl get pods -n pairdb
echo ""
echo "Services:"
kubectl get svc -n pairdb
echo ""
echo -e "${GREEN}To access the API Gateway:${NC}"
echo "  kubectl port-forward -n pairdb svc/api-gateway 8080:8080"
echo ""
echo -e "${GREEN}To view logs:${NC}"
echo "  kubectl logs -f -l app.kubernetes.io/name=api-gateway -n pairdb"
echo "  kubectl logs -f -l app=coordinator -n pairdb"
echo "  kubectl logs -f -l app=pairdb-storage-node -n pairdb"
echo ""
echo -e "${YELLOW}Note: Make sure Docker images are built and loaded into Minikube:${NC}"
echo "  See local_setup/README.md for image building instructions"

