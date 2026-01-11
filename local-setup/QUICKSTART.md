# PairDB Local Setup - Quick Start Guide

Get PairDB running locally in under 5 minutes!

## Prerequisites

- Minikube installed
- kubectl installed
- Docker installed
- Go 1.24+ (for building from source)
- Python 3.8+ (for running tests)

## Step 1: Start Minikube

```bash
minikube start --cpus=4 --memory=8192
```

## Step 2: Build and Load Images

```bash
cd /Users/froz.husain/go/devrev.horizon.cloud/pairDB

# Build coordinator
cd coordinator
make docker-build
cd ..

# Build storage-node
cd storage-node
make docker-build
cd ..

# Build API gateway
cd api-gateway
make docker-build
cd ..

# Load all images into Minikube
cd local-setup
./scripts/load-images.sh
```

## Step 3: Deploy PairDB

```bash
# Create namespace
kubectl apply -f k8s/namespace.yaml

# Deploy infrastructure (PostgreSQL, Redis)
kubectl apply -f k8s/postgres/
kubectl apply -f k8s/redis/

# Wait for infrastructure to be ready (about 30 seconds)
kubectl wait --for=condition=ready pod -l app=postgres -n pairdb --timeout=120s
kubectl wait --for=condition=ready pod -l app=redis -n pairdb --timeout=120s

# Deploy application components
kubectl apply -f k8s/coordinator/
kubectl apply -f k8s/storage-node/
kubectl apply -f k8s/api-gateway/
```

## Step 4: Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n pairdb

# You should see something like:
# NAME                           READY   STATUS    RESTARTS   AGE
# api-gateway-xxx-xxx            1/1     Running   0          1m
# coordinator-xxx-xxx            1/1     Running   0          1m
# pairdb-storage-node-0          1/1     Running   0          1m
# pairdb-storage-node-1          1/1     Running   0          1m
# postgres-xxx-xxx               1/1     Running   0          2m
# redis-xxx-xxx                  1/1     Running   0          2m
```

**Note:** Storage nodes may show `0/1` Ready if health probes aren't implemented. Check they're in `Running` status.

## Step 5: Run Tests

```bash
# Run comprehensive test suite
python3 test_pairdb.py

# Open the HTML report
open pairdb_test_report.html
```

## Expected Output

```
================================================================================
                            PairDB Test Suite
================================================================================

Validating Deployment
---------------------
✓ Namespace exists (12.45ms)
✓ PostgreSQL deployed (23.12ms)
✓ Redis deployed (18.67ms)
✓ Coordinator deployed (19.34ms)
✓ Storage Node deployed (21.56ms)
✓ API Gateway deployed (17.89ms)

Checking Pod Health
-------------------
✓ Pod: postgres-xxx-xxx (15.23ms)
✓ Pod: redis-xxx-xxx (12.67ms)
⚠ Pod: coordinator-xxx-xxx (18.45ms)
  Pod is Running (container coordinator not ready)
⚠ Pod: pairdb-storage-node-0 (16.78ms)
  Pod is Running (container storage-node not ready)

... (more tests)

================================================================================
                              Test Summary
================================================================================

Overall Results:
  Total Tests:   25
  ✓ Passed:      23
  ✗ Failed:      0
  ○ Skipped:     2
```

## Common Issues

### ImagePullBackOff

**Problem:** Pods can't pull images

**Solution:**
```bash
./scripts/load-images.sh
kubectl delete pods -n pairdb --all  # Restart all pods
```

### CrashLoopBackOff (Coordinator)

**Problem:** Coordinator can't connect to PostgreSQL

**Solution:**
```bash
# Check PostgreSQL is running
kubectl get pods -n pairdb -l app=postgres

# Check logs
kubectl logs -n pairdb -l app=coordinator --tail=50

# Verify secrets exist
kubectl get secrets -n pairdb coordinator-secrets
```

### Port Already in Use

**Problem:** Port-forward fails

**Solution:**
```bash
# Kill existing port-forwards
pkill -f "port-forward.*8080"

# Or use a different port
kubectl port-forward -n pairdb svc/api-gateway 8081:8080
```

## Manual Testing

If you want to test manually without the script:

```bash
# Port-forward to API Gateway
kubectl port-forward -n pairdb svc/api-gateway 8080:8080

# In another terminal, test health endpoint
curl http://localhost:8080/health

# Write a key-value pair
curl -X POST http://localhost:8080/v1/key-value \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "test-tenant",
    "key": "test-key",
    "value": "test-value",
    "consistency": "quorum"
  }'

# Read the key-value pair
curl "http://localhost:8080/v1/key-value?tenant_id=test-tenant&key=test-key&consistency=quorum"
```

## Cleanup

To remove everything:

```bash
kubectl delete namespace pairdb
```

Or use the cleanup script:

```bash
./scripts/cleanup-local.sh
```

## Next Steps

1. ✅ **Read the full testing guide**: [TESTING.md](TESTING.md)
2. 📖 **Learn about the architecture**: [../docs/README.md](../docs/README.md)
3. 🔧 **Customize your setup**: Edit files in `k8s/` directory
4. 📊 **Monitor performance**: Check the HTML test report
5. 🐛 **Debug issues**: Use `kubectl logs` and `kubectl describe`

## Resources

- **Main README**: [../README.md](../README.md)
- **Testing Guide**: [TESTING.md](TESTING.md)
- **Local Setup Details**: [README.md](README.md)
- **High-Level Design**: [../docs/high-level-design.md](../docs/high-level-design.md)

## Getting Help

Check logs for any component:

```bash
# Coordinator
kubectl logs -n pairdb -l app=coordinator -f

# Storage Node
kubectl logs -n pairdb -l app=pairdb-storage-node -f

# API Gateway
kubectl logs -n pairdb -l app.kubernetes.io/name=api-gateway -f

# PostgreSQL
kubectl logs -n pairdb -l app=postgres -f

# Redis
kubectl logs -n pairdb -l app=redis -f
```

Describe a pod for detailed information:

```bash
kubectl describe pod -n pairdb <pod-name>
```

Check events:

```bash
kubectl get events -n pairdb --sort-by='.lastTimestamp'
```

---

**Happy Testing! 🚀**
