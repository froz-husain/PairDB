# Local Development Setup for pairDB

This directory contains scripts and configurations to deploy pairDB on a local Kubernetes cluster (Minikube) with minimal resource requirements suitable for development and testing.

## Prerequisites

- **Minikube** installed and running
- **kubectl** configured to use Minikube
- **Docker** (Minikube will use it)
- At least **4GB RAM** and **2 CPU cores** available for Minikube

## Quick Start

1. **Start Minikube** (if not already running):
   ```bash
   minikube start --memory=4096 --cpus=2
   ```

2. **Enable required addons**:
   ```bash
   minikube addons enable ingress
   ```

3. **Deploy all components**:
   ```bash
   ./deploy.sh
   ```

4. **Verify deployment**:
   ```bash
   kubectl get pods -n pairdb
   ```

## What Gets Deployed

This local setup deploys:

1. **PostgreSQL** - Metadata store (single instance, small resources)
2. **Redis** - Cache and idempotency store (single instance, small resources)
3. **Storage Nodes** - 2 replicas (StatefulSet with 100MB storage each)
4. **Coordinator** - 2 replicas (Deployment)
5. **API Gateway** - 2 replicas (Deployment)

## Resource Requirements

### Per Component (Local Setup)

| Component | CPU Request | Memory Request | CPU Limit | Memory Limit | Storage |
|-----------|-------------|----------------|-----------|--------------|---------|
| PostgreSQL | 100m | 256Mi | 500m | 512Mi | 1Gi |
| Redis | 50m | 128Mi | 200m | 256Mi | - |
| Storage Node | 100m | 256Mi | 500m | 512Mi | 100Mi |
| Coordinator | 100m | 128Mi | 500m | 256Mi | - |
| API Gateway | 50m | 64Mi | 200m | 128Mi | - |

**Total Resources (approximate)**:
- CPU: ~1 core
- Memory: ~1.5GB
- Storage: ~2.5GB

## Directory Structure

```
local_setup/
├── README.md                    # This file
├── deploy.sh                    # Main deployment script
├── cleanup.sh                   # Cleanup script
├── k8s/                         # Kubernetes manifests
│   ├── namespace.yaml           # Namespace definition
│   ├── postgres/                # PostgreSQL deployment
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── pvc.yaml
│   ├── redis/                   # Redis deployment
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── storage-node/            # Storage node manifests
│   │   ├── statefulset.yaml
│   │   ├── service.yaml
│   │   ├── service-headless.yaml
│   │   └── configmap.yaml
│   ├── coordinator/             # Coordinator manifests
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── configmap.yaml
│   │   └── secrets.yaml
│   └── api-gateway/             # API Gateway manifests
│       ├── deployment.yaml
│       ├── service.yaml
│       └── configmap.yaml
└── scripts/                     # Helper scripts
    ├── init-db.sh               # Database initialization
    └── wait-for-services.sh      # Wait for services to be ready
```

## Deployment Steps

### 1. Build Docker Images

First, build the Docker images for all components:

```bash
# Build API Gateway
cd ../api-gateway
make docker
minikube image load pairdb/api-gateway:latest

# Build Coordinator
cd ../coordinator
make docker-build
minikube image load pairdb/coordinator:latest

# Build Storage Node
cd ../storage-node
make docker-build
minikube image load pairdb/storage-node:latest
```

Or use the provided script:

```bash
./scripts/build-images.sh
```

### 2. Deploy Infrastructure

Deploy PostgreSQL and Redis:

```bash
kubectl apply -f k8s/postgres/
kubectl apply -f k8s/redis/
```

Wait for them to be ready:

```bash
kubectl wait --for=condition=ready pod -l app=postgres -n pairdb --timeout=120s
kubectl wait --for=condition=ready pod -l app=redis -n pairdb --timeout=120s
```

### 3. Initialize Database

Run database migrations:

```bash
# Get PostgreSQL pod name
PG_POD=$(kubectl get pod -l app=postgres -n pairdb -o jsonpath='{.items[0].metadata.name}')

# Copy migration file
kubectl cp ../coordinator/migrations/001_initial_schema.sql pairdb/$PG_POD:/tmp/

# Execute migration
kubectl exec -n pairdb $PG_POD -- psql -U coordinator -d pairdb_metadata -f /tmp/001_initial_schema.sql
```

Or use the provided script:

```bash
./scripts/init-db.sh
```

### 4. Deploy Application Components

Deploy in order:

```bash
# 1. Storage Nodes (StatefulSet)
kubectl apply -f k8s/storage-node/

# 2. Coordinator
kubectl apply -f k8s/coordinator/

# 3. API Gateway
kubectl apply -f k8s/api-gateway/
```

Or use the main deployment script:

```bash
./deploy.sh
```

## Accessing Services

### Port Forwarding

```bash
# API Gateway
kubectl port-forward -n pairdb svc/api-gateway 8080:8080

# Coordinator (if needed)
kubectl port-forward -n pairdb svc/coordinator 50051:50051

# PostgreSQL (if needed)
kubectl port-forward -n pairdb svc/postgres 5432:5432

# Redis (if needed)
kubectl port-forward -n pairdb svc/redis 6379:6379
```

### Using Minikube Service

```bash
# Get API Gateway URL
minikube service api-gateway -n pairdb --url
```

## Testing the Deployment

### 1. Check Pod Status

```bash
kubectl get pods -n pairdb
```

All pods should be in `Running` state.

### 2. Check Service Endpoints

```bash
kubectl get svc -n pairdb
```

### 3. Test API Gateway

```bash
# Create a tenant
curl -X POST http://localhost:8080/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "test-tenant",
    "replication_factor": 2
  }'

# Write a key-value pair
curl -X POST http://localhost:8080/v1/key-value \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "test-tenant",
    "key": "test-key",
    "value": "test-value",
    "consistency": "quorum"
  }'

# Read a key-value pair
curl "http://localhost:8080/v1/key-value?tenant_id=test-tenant&key=test-key&consistency=quorum"
```

## Monitoring

### View Logs

```bash
# API Gateway logs
kubectl logs -f -l app.kubernetes.io/name=api-gateway -n pairdb

# Coordinator logs
kubectl logs -f -l app=coordinator -n pairdb

# Storage Node logs
kubectl logs -f -l app=pairdb-storage-node -n pairdb
```

### View Metrics

```bash
# Port forward metrics endpoints
kubectl port-forward -n pairdb svc/api-gateway 9090:9090
curl http://localhost:9090/metrics
```

## Troubleshooting

### Pods Not Starting

1. Check pod status:
   ```bash
   kubectl describe pod <pod-name> -n pairdb
   ```

2. Check events:
   ```bash
   kubectl get events -n pairdb --sort-by='.lastTimestamp'
   ```

3. Check resource availability:
   ```bash
   minikube status
   kubectl top nodes
   ```

### Database Connection Issues

1. Verify PostgreSQL is running:
   ```bash
   kubectl get pod -l app=postgres -n pairdb
   ```

2. Check database logs:
   ```bash
   kubectl logs -l app=postgres -n pairdb
   ```

3. Test connection:
   ```bash
   kubectl exec -it -n pairdb $(kubectl get pod -l app=postgres -n pairdb -o jsonpath='{.items[0].metadata.name}') -- psql -U coordinator -d pairdb_metadata
   ```

### Storage Node Issues

1. Check persistent volumes:
   ```bash
   kubectl get pvc -n pairdb
   ```

2. Check storage node logs:
   ```bash
   kubectl logs -l app=pairdb-storage-node -n pairdb
   ```

## Cleanup

To remove all deployed resources:

```bash
./cleanup.sh
```

Or manually:

```bash
kubectl delete namespace pairdb
```

## Configuration

All configurations are in the `k8s/` directory. Key configuration files:

- **ConfigMaps**: Non-sensitive configuration
- **Secrets**: Sensitive data (passwords, etc.)
- **Deployments/StatefulSets**: Application definitions with resource limits

## Resource Scaling

To increase resources for local development:

1. Edit the manifests in `k8s/` directory
2. Update resource requests/limits
3. Reapply:
   ```bash
   kubectl apply -f k8s/
   ```

## Notes

- This setup is **NOT** for production use
- Storage is minimal (100MB per storage node)
- Replication factors are reduced (2 replicas instead of 3)
- No persistent backups or disaster recovery
- Single-node PostgreSQL and Redis (no high availability)

## Next Steps

After successful deployment:

1. Review component logs
2. Test API endpoints
3. Monitor resource usage
4. Scale components as needed
5. Explore the [main README](../README.md) for more details

