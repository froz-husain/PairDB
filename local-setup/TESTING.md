# PairDB Testing Guide

This guide explains how to use the comprehensive test suite for PairDB local deployment.

## Test Script Overview

The `test_pairdb.py` script provides automated testing for your PairDB deployment with the following capabilities:

### Test Categories

1. **Deployment Validation**
   - Verifies namespace exists
   - Checks all components are deployed (PostgreSQL, Redis, Coordinator, Storage Node, API Gateway)
   - Validates resource existence

2. **Pod Health Checks**
   - Monitors pod status (Running, Pending, Failed)
   - Checks container readiness
   - Reports pod phase and container statuses

3. **Service Health**
   - Tests API Gateway health endpoint
   - Validates service connectivity
   - Sets up automatic port-forwarding

4. **Functional Tests**
   - **CRUD Operations**: Create, Read, Update, Delete
   - **Data Consistency**: Validates written data matches read data
   - **Key-Value Operations**: Tests basic storage operations

5. **Performance Tests**
   - **Write Latency**: Measures write operation performance (avg, p50, p95, p99)
   - **Read Latency**: Measures read operation performance
   - **Throughput**: Operations per second
   - **Concurrent Operations**: Tests multi-threaded write operations

6. **Report Generation**
   - Beautiful HTML report with charts and statistics
   - Console output with color-coded results
   - Detailed metrics and performance data

## Prerequisites

- Python 3.8 or higher
- kubectl configured for your Minikube cluster
- PairDB deployed on Minikube (run `./scripts/deploy-local.sh` first)
- Port 8080 available for port-forwarding

## Quick Start

### 1. Basic Test Run

Run all tests with default settings:

```bash
cd /Users/froz.husain/go/devrev.horizon.cloud/pairDB/local-setup
python3 test_pairdb.py
```

This will:
- Validate the deployment
- Check pod health
- Test CRUD operations
- Run performance benchmarks
- Generate an HTML report: `pairdb_test_report.html`

### 2. Custom Options

```bash
# Specify different namespace
python3 test_pairdb.py --namespace my-pairdb

# Skip deployment validation (if you know it's already working)
python3 test_pairdb.py --skip-validation

# Custom output report name
python3 test_pairdb.py --output my_report.html

# Use custom API Gateway URL (skip port-forwarding)
python3 test_pairdb.py --api-gateway-url http://localhost:8080
```

### 3. View Results

The script provides two types of output:

**Console Output:**
```
================================================================================
                            PairDB Test Suite
================================================================================

Validating Deployment
---------------------
✓ Namespace exists (12.45ms)
  Namespace 'pairdb' found
✓ PostgreSQL deployed (23.12ms)
  deployment/postgres exists
✓ Redis deployed (18.67ms)
  deployment/redis exists
...

================================================================================
                              Test Summary
================================================================================

Overall Results:
  Total Tests:   25
  ✓ Passed:      23
  ✗ Failed:      1
  ○ Skipped:     1
```

**HTML Report:**
Open the generated HTML file in your browser:
```bash
open pairdb_test_report.html
```

The HTML report includes:
- Summary dashboard with pass/fail statistics
- Detailed results for each test suite
- Performance metrics with latency percentiles
- Visual color-coding for easy identification
- Expandable test details with JSON data

## Test Descriptions

### Deployment Validation Tests

**What it checks:**
- Kubernetes namespace exists
- All required deployments are present
- StatefulSets are configured

**Pass criteria:**
- All components are deployed and accessible via kubectl

**Typical duration:** 1-3 seconds

---

### Pod Health Check Tests

**What it checks:**
- Pod phase (Running, Pending, CrashLoopBackOff, etc.)
- Container readiness status
- Resource allocation

**Pass criteria:**
- Pods are in "Running" phase
- Containers are ready (note: some services may show as not ready if health probes aren't implemented)

**Typical duration:** 1-2 seconds

**Note:** Storage nodes may show as "Running" but not "Ready" if HTTP health endpoints aren't implemented (gRPC-only services). This is expected.

---

### Service Health Check Tests

**What it checks:**
- API Gateway `/health` endpoint responds
- Port-forwarding is working
- Service is accessible

**Pass criteria:**
- HTTP 200 response from health endpoint

**Typical duration:** 2-5 seconds

---

### CRUD Operations Tests

**What it checks:**
- **Write**: Can create new key-value pairs
- **Read**: Can retrieve stored values
- **Update**: Can modify existing values
- **Delete**: Can remove keys

**Pass criteria:**
- All operations return success
- Read values match written values

**Typical duration:** 5-10 seconds

**Data used:**
- Generates unique tenant IDs and keys for each test run
- Uses timestamp-based values to ensure uniqueness
- Cleans up after itself with delete operations

---

### Write Latency Tests

**What it checks:**
- Average write latency
- Latency distribution (P50, P95, P99)
- Throughput (operations per second)

**Pass criteria:**
- Writes complete successfully
- Latency is within acceptable range

**Typical duration:** 10-20 seconds (for 50-100 operations)

**Metrics reported:**
- Average latency (ms)
- Minimum latency (ms)
- Maximum latency (ms)
- P50 (median) latency (ms)
- P95 latency (ms)
- P99 latency (ms)
- Throughput (ops/sec)

**Expected performance:**
- Write latency: 10-100ms (depends on hardware)
- Throughput: 10-100 ops/sec for sequential writes

---

### Read Latency Tests

**What it checks:**
- Average read latency
- Latency distribution
- Read throughput

**Pass criteria:**
- Reads complete successfully
- Data retrieved matches written data

**Typical duration:** 10-20 seconds

**Metrics reported:**
- Same as write latency tests

**Expected performance:**
- Read latency: typically 5-50ms (faster than writes)
- Throughput: 20-200 ops/sec for sequential reads

---

### Concurrent Write Tests

**What it checks:**
- System behavior under concurrent load
- Thread safety
- Consistency under concurrent writes

**Pass criteria:**
- All concurrent writes succeed
- No data corruption
- Acceptable latency under load

**Typical duration:** 15-30 seconds

**Configuration:**
- Default: 5 threads, 10 operations per thread
- Total: 50 concurrent operations

**Metrics reported:**
- Total successful operations
- Total failed operations
- Average latency under load
- P95 latency
- Throughput under concurrency

**Expected performance:**
- Success rate: 90-100%
- Latency may increase by 20-50% compared to sequential operations

## Interpreting Results

### Exit Codes

The script uses standard exit codes:
- `0`: All tests passed
- `1`: One or more tests failed
- `130`: User interrupted (Ctrl+C)

This makes it suitable for CI/CD pipelines:
```bash
if python3 test_pairdb.py; then
    echo "All tests passed!"
else
    echo "Tests failed!"
    exit 1
fi
```

### Test Status

- **PASSED** (✓): Test completed successfully
- **FAILED** (✗): Test encountered an error or didn't meet criteria
- **WARNING** (⚠): Test passed but with warnings (e.g., some operations failed)
- **SKIPPED** (○): Test was skipped (e.g., API not available)

### Common Issues

#### "ImagePullBackOff" Errors

**Symptom:** Pod health checks fail with ImagePullBackOff

**Solution:**
```bash
cd /Users/froz.husain/go/devrev.horizon.cloud/pairDB/local-setup
./scripts/load-images.sh
```

This loads Docker images into Minikube's Docker daemon.

---

#### "Port-forward Failed"

**Symptom:** Service health checks fail, port-forward errors

**Solution:**
1. Check if API Gateway is running:
   ```bash
   kubectl get pods -n pairdb -l app.kubernetes.io/name=api-gateway
   ```

2. Manually test port-forward:
   ```bash
   kubectl port-forward -n pairdb svc/api-gateway 8080:8080
   ```

3. Ensure port 8080 is not already in use:
   ```bash
   lsof -i :8080
   ```

---

#### "All operations failed"

**Symptom:** CRUD or performance tests fail completely

**Possible causes:**
1. API Gateway not responding
2. Coordinator can't connect to storage nodes
3. Database connection issues

**Debug steps:**
```bash
# Check coordinator logs
kubectl logs -n pairdb -l app=coordinator --tail=50

# Check storage node logs
kubectl logs -n pairdb -l app=pairdb-storage-node --tail=50

# Check API gateway logs
kubectl logs -n pairdb -l app.kubernetes.io/name=api-gateway --tail=50
```

---

#### "CrashLoopBackOff" Status

**Symptom:** Coordinator pods in CrashLoopBackOff

**Common causes:**
1. Database connection issues
2. Missing secrets
3. Configuration errors

**Solution:**
```bash
# Check if PostgreSQL is running
kubectl get pods -n pairdb -l app=postgres

# Check coordinator secrets
kubectl get secrets -n pairdb coordinator-secrets

# View detailed error logs
kubectl logs -n pairdb <coordinator-pod-name>
```

---

#### High Latency / Low Throughput

**Symptom:** Performance tests pass but with poor metrics

**Possible causes:**
1. Resource constraints (CPU/memory limits)
2. Network latency in Minikube
3. Storage performance issues

**Debugging:**
```bash
# Check resource usage
kubectl top pods -n pairdb

# Check node resources
kubectl top nodes

# Increase Minikube resources
minikube stop
minikube start --cpus=4 --memory=8192
```

## Advanced Usage

### Running Specific Test Categories

The script currently runs all tests. To run specific tests, you can modify the script or create wrapper scripts:

```bash
# Example: Only validation and health checks
# (Modify run_all_tests() method to comment out performance tests)
```

### Customizing Test Parameters

Edit the `test_pairdb.py` script to adjust:

```python
# Performance test parameters
self.test_write_latency(num_operations=100)  # Increase to 100
self.test_read_latency(num_operations=100)
self.test_concurrent_writes(num_threads=10, ops_per_thread=20)  # More load
```

### Integration with CI/CD

Example GitLab CI configuration:

```yaml
test:
  stage: test
  script:
    - cd local-setup
    - python3 test_pairdb.py --output report.html
  artifacts:
    paths:
      - local-setup/report.html
    when: always
  allow_failure: false
```

Example GitHub Actions:

```yaml
- name: Run PairDB Tests
  run: |
    cd local-setup
    python3 test_pairdb.py

- name: Upload Test Report
  uses: actions/upload-artifact@v3
  with:
    name: test-report
    path: local-setup/pairdb_test_report.html
  if: always()
```

## Performance Benchmarks

Expected performance on typical development machines (Minikube with 4 CPU, 8GB RAM):

| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| Write Latency (avg) | < 50ms | 50-150ms | > 150ms |
| Read Latency (avg) | < 30ms | 30-100ms | > 100ms |
| Write Throughput | > 50 ops/sec | 20-50 ops/sec | < 20 ops/sec |
| Read Throughput | > 100 ops/sec | 50-100 ops/sec | < 50 ops/sec |
| Concurrent Write Success Rate | 100% | 95-99% | < 95% |

**Note:** These are guidelines for local development. Production performance will differ based on hardware and network.

## Troubleshooting

### Script Errors

**ImportError: No module named 'urllib'**
- Solution: Use Python 3.8+ (urllib is built-in)

**Permission denied**
- Solution: `chmod +x test_pairdb.py`

**kubectl: command not found**
- Solution: Install kubectl and configure for Minikube

### Test Timeouts

If tests timeout frequently:

1. Increase timeout values in script:
   ```python
   response = urllib.request.urlopen(req, timeout=30)  # Increase from 10
   ```

2. Check system resources:
   ```bash
   minikube status
   kubectl top nodes
   ```

## Next Steps

After running tests:

1. **Review the HTML report** for detailed insights
2. **Check failed tests** and investigate root causes
3. **Monitor pod logs** for any errors or warnings
4. **Tune performance** based on benchmark results
5. **Run tests regularly** to catch regressions

## Contributing

To add new tests:

1. Add a new test method to the `PairDBTester` class:
   ```python
   def test_my_new_feature(self) -> TestSuite:
       suite = TestSuite(name="My New Feature")
       # Add test logic
       self.suites.append(suite)
       return suite
   ```

2. Call it from `run_all_tests()`:
   ```python
   self.test_my_new_feature()
   ```

3. Update this documentation with test details

## Support

For issues or questions:
- Check pod logs: `kubectl logs -n pairdb <pod-name>`
- Review deployment: `kubectl describe pod -n pairdb <pod-name>`
- See main README: [../README.md](../README.md)
- Local setup guide: [README.md](README.md)
