#!/bin/bash

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo -e "${GREEN}=== Initializing PearDB Database ===${NC}"

# Get PostgreSQL pod
PG_POD=$(kubectl get pod -l app=postgres -n pairdb -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$PG_POD" ]; then
    echo -e "${RED}PostgreSQL pod not found. Make sure PostgreSQL is deployed.${NC}"
    exit 1
fi

echo "Using PostgreSQL pod: $PG_POD"

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres -n pairdb --timeout=120s || {
    echo -e "${RED}PostgreSQL is not ready${NC}"
    exit 1
}

# Wait a bit more for PostgreSQL to accept connections
sleep 5

# Check if migration already exists
echo "Checking if database is already initialized..."
if kubectl exec -n pairdb "$PG_POD" -- psql -U coordinator -d pairdb_metadata -c "\dt" 2>/dev/null | grep -q tenants; then
    echo -e "${YELLOW}Database already initialized.${NC}"
    exit 0
fi

# Run migration
MIGRATION_FILE="${ROOT_DIR}/coordinator/migrations/001_initial_schema.sql"

if [ ! -f "$MIGRATION_FILE" ]; then
    echo -e "${RED}Migration file not found at: $MIGRATION_FILE${NC}"
    exit 1
fi

echo "Copying migration file to pod..."
kubectl cp "$MIGRATION_FILE" "pairdb/$PG_POD:/tmp/001_initial_schema.sql"

echo "Running migration..."
kubectl exec -n pairdb "$PG_POD" -- psql -U coordinator -d pairdb_metadata -f /tmp/001_initial_schema.sql

echo ""
echo -e "${GREEN}=== Database Initialization Complete ===${NC}"

