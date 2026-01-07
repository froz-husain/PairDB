# Deployment Guide

## Overview

This guide covers deploying PairDB Storage Node to Kubernetes in production.

## Prerequisites

### Infrastructure
- Kubernetes cluster version >= 1.24
- kubectl configured with cluster access
- Sufficient cluster resources:
  - 3 nodes minimum (for pod anti-affinity)
  - 6 CPU cores available (2 per pod)
  - 12 GB RAM available (4 GB per pod)
  - 300 GB storage (100 GB per pod)

### Tools
- `kubectl` v1.24+
- `helm` v3.0+ (optional)
- Docker (for building images)

### Storage
- StorageClass `fast-ssd` configured with SSD-backed storage
- Support for ReadWriteOnce volumes

## Quick Start

### 1. Build Docker Image

```bash
# Build the image
docker build -t pairdb/storage-node:v1.0.0 .

# Tag for your registry
docker tag pairdb/storage-node:v1.0.0 your-registry/pairdb/storage-node:v1.0.0

# Push to registry
docker push your-registry/pairdb/storage-node:v1.0.0
```

### 2. Deploy to Kubernetes

```bash
# Create namespace
kubectl apply -f deployments/k8s/namespace.yaml

# Deploy all components
kubectl apply -f deployments/k8s/

# Verify deployment
kubectl get pods -n pairdb
kubectl get pvc -n pairdb
```

## Detailed Deployment Steps

### Step 1: Prepare Namespace

```bash
kubectl apply -f deployments/k8s/namespace.yaml

# Verify
kubectl get namespace pairdb
```

### Step 2: Create RBAC Resources

```bash
kubectl apply -f deployments/k8s/serviceaccount.yaml

# Verify
kubectl get serviceaccount -n pairdb
kubectl get role -n pairdb
kubectl get rolebinding -n pairdb
```

### Step 3: Deploy ConfigMap

Review and customize configuration:

```bash
# Edit config if needed
vim deployments/k8s/configmap.yaml

# Apply
kubectl apply -f deployments/k8s/configmap.yaml

# Verify
kubectl get configmap -n pairdb
kubectl describe configmap pairdb-storage-node-config -n pairdb
```

### Step 4: Deploy Services

```bash
# Deploy headless service (for gossip)
kubectl apply -f deployments/k8s/service-headless.yaml

# Deploy regular service (for gRPC)
kubectl apply -f deployments/k8s/service.yaml

# Verify
kubectl get svc -n pairdb
```

### Step 5: Create PodDisruptionBudget

```bash
kubectl apply -f deployments/k8s/pdb.yaml

# Verify
kubectl get pdb -n pairdb
```

### Step 6: Deploy StatefulSet

**IMPORTANT**: Update the image in `statefulset.yaml` before deploying:

```yaml
containers:
- name: storage-node
  image: your-registry/pairdb/storage-node:v1.0.0  # Update this
```

Deploy:

```bash
kubectl apply -f deployments/k8s/statefulset.yaml

# Watch pods come up
kubectl get pods -n pairdb -w

# Check status
kubectl get statefulset -n pairdb
```

### Step 7: Verify Deployment

```bash
# Check pod status
kubectl get pods -n pairdb

# Check PVCs
kubectl get pvc -n pairdb

# Check logs
kubectl logs -n pairdb pairdb-storage-node-0

# Check health
kubectl exec -n pairdb pairdb-storage-node-0 -- wget -qO- http://localhost:9091/health
```

## Configuration

### Resource Sizing

Adjust resources based on your workload:

```yaml
resources:
  requests:
    cpu: "500m"        # Minimum CPU
    memory: "1Gi"      # Minimum memory
  limits:
    cpu: "2000m"       # Maximum CPU
    memory: "4Gi"      # Maximum memory
```

### Storage Sizing

```yaml
volumeClaimTemplates:
  - spec:
      resources:
        requests:
          storage: 100Gi  # Adjust based on data volume
```

### Replica Count

```yaml
spec:
  replicas: 3  # Minimum 3 for high availability
```

## Monitoring Setup

### Prometheus ServiceMonitor

If using Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: pairdb-storage-node
  namespace: pairdb
spec:
  selector:
    matchLabels:
      app: pairdb-storage-node
  endpoints:
  - port: metrics
    interval: 30s
```

Apply:
```bash
kubectl apply -f prometheus-servicemonitor.yaml
```

### Alert Rules

```bash
kubectl apply -f deployments/monitoring/alerts.yaml
```

## Health Checks

### Liveness Probe
Checks if the process is responsive:
```
GET /health/live on port 9091
```

### Readiness Probe
Checks if the node can serve traffic:
```
GET /health/ready on port 9091
```

Test manually:
```bash
kubectl exec -n pairdb pairdb-storage-node-0 -- \
  wget -qO- http://localhost:9091/health/ready
```

## Scaling

### Scale Up

```bash
# Scale to 5 replicas
kubectl scale statefulset pairdb-storage-node -n pairdb --replicas=5

# Verify
kubectl get pods -n pairdb
```

### Scale Down

```bash
# Scale to 3 replicas
kubectl scale statefulset pairdb-storage-node -n pairdb --replicas=3

# Drain node first (recommended)
kubectl exec -n pairdb pairdb-storage-node-4 -- /app/drain-node

# Then scale
kubectl scale statefulset pairdb-storage-node -n pairdb --replicas=3
```

## Updates and Rolling Deploys

### Rolling Update

```bash
# Update image
kubectl set image statefulset/pairdb-storage-node \
  storage-node=your-registry/pairdb/storage-node:v1.1.0 \
  -n pairdb

# Monitor rollout
kubectl rollout status statefulset/pairdb-storage-node -n pairdb

# Check rollout history
kubectl rollout history statefulset/pairdb-storage-node -n pairdb
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo statefulset/pairdb-storage-node -n pairdb

# Rollback to specific revision
kubectl rollout undo statefulset/pairdb-storage-node -n pairdb --to-revision=2
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod pairdb-storage-node-0 -n pairdb

# Check events
kubectl get events -n pairdb --sort-by='.lastTimestamp'

# Check logs
kubectl logs pairdb-storage-node-0 -n pairdb

# Check previous logs (if crashed)
kubectl logs pairdb-storage-node-0 -n pairdb --previous
```

### PVC Issues

```bash
# Check PVC status
kubectl get pvc -n pairdb

# Describe PVC
kubectl describe pvc data-pairdb-storage-node-0 -n pairdb

# Check StorageClass
kubectl get storageclass fast-ssd
```

### Network Issues

```bash
# Test service connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -n pairdb -- \
  wget -qO- http://pairdb-storage-node:8080

# Test from another pod
kubectl exec -n pairdb pairdb-storage-node-0 -- \
  nc -zv pairdb-storage-node-1.pairdb-storage-node-headless 8080
```

### Health Check Failures

```bash
# Check readiness
kubectl get pods -n pairdb -o wide

# Test health endpoint
kubectl exec -n pairdb pairdb-storage-node-0 -- \
  wget -qO- http://localhost:9091/health/ready

# Check disk space
kubectl exec -n pairdb pairdb-storage-node-0 -- df -h /data
```

## Backup and Restore

### Backup Data

```bash
# Backup PVC data
kubectl exec -n pairdb pairdb-storage-node-0 -- \
  tar czf /tmp/backup.tar.gz /data

# Copy to local
kubectl cp pairdb/pairdb-storage-node-0:/tmp/backup.tar.gz ./backup.tar.gz
```

### Restore Data

```bash
# Copy backup to pod
kubectl cp ./backup.tar.gz pairdb/pairdb-storage-node-0:/tmp/backup.tar.gz

# Restore
kubectl exec -n pairdb pairdb-storage-node-0 -- \
  tar xzf /tmp/backup.tar.gz -C /
```

## Cleanup

### Delete Deployment

```bash
# Delete StatefulSet (keeps PVCs)
kubectl delete statefulset pairdb-storage-node -n pairdb

# Delete services
kubectl delete svc pairdb-storage-node pairdb-storage-node-headless -n pairdb

# Delete PVCs (WARNING: deletes data)
kubectl delete pvc -n pairdb -l app=pairdb-storage-node

# Delete namespace (deletes everything)
kubectl delete namespace pairdb
```

## Production Checklist

Before going to production:

- [ ] Container image built and pushed to registry
- [ ] Resources (CPU, memory, storage) sized appropriately
- [ ] StorageClass configured with SSD storage
- [ ] Monitoring and alerting configured
- [ ] Backup procedures established
- [ ] Disaster recovery plan documented
- [ ] Load testing completed
- [ ] Security review completed
- [ ] TLS/mTLS configured (if required)
- [ ] Network policies applied
- [ ] Resource quotas set
- [ ] On-call team trained

## Multi-Region Deployment

For multi-region setup:

1. Deploy to each region independently
2. Configure cross-region replication (coordinator layer)
3. Set up global load balancing
4. Configure region-specific monitoring
5. Plan for regional failover

## Security Considerations

### Pod Security

- Runs as non-root user (UID 1000)
- Read-only root filesystem (except data volumes)
- Capabilities dropped
- Security contexts enforced

### Network Security

```yaml
# Example NetworkPolicy
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: pairdb-storage-node-netpol
  namespace: pairdb
spec:
  podSelector:
    matchLabels:
      app: pairdb-storage-node
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: pairdb-coordinator
    ports:
    - protocol: TCP
      port: 8080
```

## Support

For deployment issues:
- Check logs: `kubectl logs -n pairdb <pod-name>`
- Check events: `kubectl get events -n pairdb`
- Review metrics: Check Prometheus/Grafana
- Consult PRODUCTION_READINESS.md
