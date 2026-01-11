# PairDB Test Script - Summary

## Overview

A comprehensive Python-based testing suite for PairDB that validates deployment, runs functional tests, measures performance, and generates detailed HTML reports.

## Files Created

### 1. `test_pairdb.py` - Main Test Script

**Location:** `/Users/froz.husain/go/devrev.horizon.cloud/pairDB/local-setup/test_pairdb.py`

**Size:** ~1,200 lines of Python code

**Features:**
- ✅ Deployment validation
- ✅ Pod health monitoring
- ✅ Service health checks
- ✅ CRUD operations testing
- ✅ Performance benchmarking (latency, throughput)
- ✅ Concurrent operations testing
- ✅ HTML report generation
- ✅ Color-coded console output
- ✅ Automatic port-forwarding
- ✅ Error handling and cleanup

### 2. `TESTING.md` - Comprehensive Testing Guide

**Location:** `/Users/froz.husain/go/devrev.horizon.cloud/pairDB/local-setup/TESTING.md`

**Contents:**
- Test categories explanation
- Prerequisites and setup
- Usage examples with all command-line options
- Detailed test descriptions with expected performance
- Result interpretation guide
- Common issues and solutions
- Advanced usage patterns
- CI/CD integration examples

### 3. `QUICKSTART.md` - Quick Reference Guide

**Location:** `/Users/froz.husain/go/devrev.horizon.cloud/pairDB/local-setup/QUICKSTART.md`

**Contents:**
- 5-step deployment guide
- Common issues and quick fixes
- Manual testing commands
- Cleanup instructions
- Resource links

## Test Script Capabilities

### Test Suites Included

#### 1. **Deployment Validation Suite**
```python
✓ Namespace exists
✓ PostgreSQL deployed
✓ Redis deployed
✓ Coordinator deployed
✓ Storage Node deployed
✓ API Gateway deployed
```

**What it validates:**
- All Kubernetes resources are created
- Deployments and StatefulSets exist
- Resources are in the correct namespace

**Duration:** 1-3 seconds

---

#### 2. **Pod Health Check Suite**
```python
✓ Pod: postgres-xxx-xxx (Running)
✓ Pod: redis-xxx-xxx (Running)
⚠ Pod: coordinator-xxx-xxx (Running, container not ready)
⚠ Pod: pairdb-storage-node-0 (Running, container not ready)
```

**What it validates:**
- Pod phases (Running, Pending, Failed, etc.)
- Container readiness status
- Resource allocation

**Duration:** 1-2 seconds

---

#### 3. **Service Health Suite**
```python
✓ Setup port-forward (3 seconds)
✓ API Gateway /health (HTTP 200)
```

**What it validates:**
- API Gateway is responding
- Health endpoint returns success
- Port-forwarding works

**Duration:** 3-5 seconds

---

#### 4. **CRUD Operations Suite**
```python
✓ Write operation (45.23ms)
✓ Read operation (23.45ms)
✓ Update operation (47.89ms)
✓ Delete operation (25.67ms)
```

**What it validates:**
- Can create key-value pairs
- Can retrieve stored data
- Can update existing values
- Can delete keys
- Data integrity (written value = read value)

**Duration:** 5-10 seconds

---

#### 5. **Write Latency Suite**
```python
✓ Write latency (100 ops in 4523.45ms)
  Avg: 45.23ms, P50: 43.12ms, P95: 78.34ms, P99: 95.67ms
  Throughput: 22.1 ops/sec
```

**Metrics measured:**
- Average latency
- Minimum/Maximum latency
- P50 (median) latency
- P95 latency (95th percentile)
- P99 latency (99th percentile)
- Throughput (operations per second)
- Error rate

**Duration:** 10-20 seconds (configurable)

---

#### 6. **Read Latency Suite**
```python
✓ Read latency (100 ops in 2345.67ms)
  Avg: 23.46ms, P50: 21.23ms, P95: 45.12ms, P99: 67.89ms
  Throughput: 42.6 ops/sec
```

**Same metrics as write latency**

**Duration:** 10-20 seconds (configurable)

---

#### 7. **Concurrent Writes Suite**
```python
✓ Concurrent writes (10 threads, 50 total ops)
  Success: 50, Errors: 0, Avg latency: 67.89ms
  Throughput: 18.5 ops/sec under concurrency
```

**What it validates:**
- Thread safety
- Consistency under concurrent load
- System stability with multiple writers

**Metrics:**
- Total operations
- Success/failure count
- Average latency under load
- P95 latency
- Throughput

**Duration:** 15-30 seconds (configurable)

---

## Usage Examples

### Basic Usage

```bash
# Run all tests with default settings
python3 test_pairdb.py

# Output:
# - Console: Color-coded test results
# - File: pairdb_test_report.html
# - Exit code: 0 (success) or 1 (failure)
```

### Advanced Usage

```bash
# Skip deployment validation (faster, assumes working deployment)
python3 test_pairdb.py --skip-validation

# Use different namespace
python3 test_pairdb.py --namespace my-pairdb

# Custom report name
python3 test_pairdb.py --output $(date +%Y%m%d)_report.html

# Use existing port-forward (no automatic setup)
python3 test_pairdb.py --api-gateway-url http://localhost:8080
```

### CI/CD Integration

```bash
# Exit code indicates pass/fail
python3 test_pairdb.py && echo "Tests passed!" || echo "Tests failed!"

# Store report as artifact
python3 test_pairdb.py --output reports/test_$(git rev-parse --short HEAD).html
```

## HTML Report Features

The generated HTML report includes:

### Summary Dashboard
```
┌─────────────────────────────────────────┐
│  Total Tests:     25                    │
│  ✓ Passed:        23                    │
│  ✗ Failed:        1                     │
│  ○ Skipped:       1                     │
└─────────────────────────────────────────┘
```

### Suite Details
- Each test suite is color-coded
- Expandable test results
- Detailed metrics in JSON format
- Timestamps and durations

### Visual Features
- **Color coding:**
  - Green = Passed
  - Red = Failed
  - Yellow = Warning
  - Gray = Skipped

- **Responsive design:** Works on desktop and mobile
- **Professional styling:** Modern UI with gradients
- **Easy navigation:** Jump to specific test suites

### Sample Report Section
```html
┌───────────────────────────────────────────────────────────┐
│ Write Latency (100 ops)                  23P  0F  0S      │
├───────────────────────────────────────────────────────────┤
│ ✓ Write latency                                 (4523ms)  │
│   Avg: 45.23ms, P50: 43.12ms, P95: 78.34ms               │
│                                                           │
│   {                                                       │
│     "operations": 100,                                    │
│     "errors": 0,                                          │
│     "avg_latency_ms": 45.23,                              │
│     "p95_latency_ms": 78.34,                              │
│     "throughput_ops_per_sec": 22.1                        │
│   }                                                       │
└───────────────────────────────────────────────────────────┘
```

## Performance Benchmarks

Expected results on typical dev machines (Minikube, 4 CPU, 8GB RAM):

### Write Performance
| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| Avg Latency | < 50ms | 50-150ms | > 150ms |
| P95 Latency | < 100ms | 100-200ms | > 200ms |
| P99 Latency | < 150ms | 150-300ms | > 300ms |
| Throughput | > 50 ops/s | 20-50 ops/s | < 20 ops/s |

### Read Performance
| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| Avg Latency | < 30ms | 30-100ms | > 100ms |
| P95 Latency | < 60ms | 60-150ms | > 150ms |
| P99 Latency | < 100ms | 100-200ms | > 200ms |
| Throughput | > 100 ops/s | 50-100 ops/s | < 50 ops/s |

### Concurrent Performance
- **Success Rate:** Should be > 95%
- **Latency Increase:** Expect 20-50% increase vs sequential
- **Stability:** No crashes or data corruption

## Customization

### Adjusting Test Parameters

Edit `test_pairdb.py` to change:

```python
# In run_all_tests() method:

# More intensive write test
self.test_write_latency(num_operations=200)  # default: 100

# More intensive read test
self.test_read_latency(num_operations=200)  # default: 100

# Higher concurrency
self.test_concurrent_writes(
    num_threads=20,      # default: 10
    ops_per_thread=20    # default: 10
)
```

### Adding Custom Tests

```python
def test_my_custom_feature(self) -> TestSuite:
    """Test custom feature"""
    suite = TestSuite(name="My Custom Feature")

    start = time.time()
    try:
        # Your test logic here
        result = self.my_test_function()

        suite.add_result(TestResult(
            name="My test",
            status=TestStatus.PASSED,
            duration_ms=(time.time() - start) * 1000,
            message="Test passed!",
            details={"result": result}
        ))
    except Exception as e:
        suite.add_result(TestResult(
            name="My test",
            status=TestStatus.FAILED,
            duration_ms=(time.time() - start) * 1000,
            message=f"Failed: {e}"
        ))

    self.suites.append(suite)
    return suite

# Add to run_all_tests():
self.test_my_custom_feature()
```

## Dependencies

The script uses **only Python standard library**, no external packages needed:
- `subprocess` - For running kubectl commands
- `json` - For parsing Kubernetes output
- `urllib` - For HTTP requests to API Gateway
- `concurrent.futures` - For concurrent test execution
- `statistics` - For calculating latency percentiles
- `argparse` - For command-line arguments
- `time`, `datetime` - For timing and timestamps
- `uuid` - For generating unique test IDs

## Error Handling

The script handles:
- ✅ Kubernetes connection failures
- ✅ Pod not found errors
- ✅ Network timeouts
- ✅ HTTP errors
- ✅ JSON parsing errors
- ✅ Port-forward failures
- ✅ Keyboard interrupts (Ctrl+C)

All errors are:
- Caught gracefully
- Logged to console
- Included in HTML report
- Properly cleaned up (port-forwards killed)

## Exit Codes

- `0` - All tests passed
- `1` - One or more tests failed
- `130` - User interrupted (Ctrl+C)

## Integration Points

### With Local Setup
```bash
# Full deployment and test flow
./scripts/deploy-local.sh    # Deploy PairDB
python3 test_pairdb.py        # Run tests
./scripts/cleanup-local.sh    # Cleanup
```

### With CI/CD
```yaml
# Example GitHub Actions
- name: Deploy PairDB
  run: cd local-setup && ./scripts/deploy-local.sh

- name: Run Tests
  run: cd local-setup && python3 test_pairdb.py

- name: Upload Report
  uses: actions/upload-artifact@v3
  with:
    name: test-report
    path: local-setup/*.html
  if: always()
```

### With Monitoring
```bash
# Run tests every hour via cron
0 * * * * cd /path/to/pairdb/local-setup && python3 test_pairdb.py --output /var/reports/$(date +\%Y\%m\%d_\%H\%M).html
```

## Troubleshooting

### Script Won't Run

**Error:** `python3: command not found`
```bash
# Install Python 3.8+
# macOS:
brew install python3

# Ubuntu:
sudo apt install python3
```

**Error:** `Permission denied`
```bash
chmod +x test_pairdb.py
```

### Tests Timing Out

**Solution:** Increase timeout in script
```python
# Line ~500 in test_pairdb.py
response = urllib.request.urlopen(req, timeout=30)  # Increase from 10
```

### Port-Forward Issues

**Error:** `port-forward failed`
```bash
# Kill existing port-forwards
pkill -f "port-forward.*8080"

# Or manually set up port-forward first
kubectl port-forward -n pairdb svc/api-gateway 8080:8080 &
python3 test_pairdb.py --api-gateway-url http://localhost:8080
```

## Future Enhancements

Possible improvements:
- [ ] Add consistency level tests (quorum vs eventual)
- [ ] Test failure scenarios (pod crashes, network partitions)
- [ ] Add stress testing mode
- [ ] Support for external metrics collection (Prometheus)
- [ ] Configurable test suites (run only specific tests)
- [ ] JSON output format for machine parsing
- [ ] Historical trend analysis
- [ ] Alert thresholds and notifications

## Summary

You now have a **production-ready test suite** that:

✅ **Validates** your PairDB deployment
✅ **Tests** all major functionality
✅ **Measures** performance with detailed metrics
✅ **Generates** beautiful HTML reports
✅ **Integrates** with CI/CD pipelines
✅ **Handles** errors gracefully
✅ **Provides** actionable insights

**Total time investment:** Script runs in 60-120 seconds
**Value delivered:** Complete confidence in your deployment

## Quick Commands Reference

```bash
# Run all tests
python3 test_pairdb.py

# Run with custom options
python3 test_pairdb.py --namespace pairdb --output my_report.html

# View help
python3 test_pairdb.py --help

# Check script is working
python3 test_pairdb.py --skip-validation  # Faster test

# Open report
open pairdb_test_report.html
```

---

**Ready to test? Run:** `python3 test_pairdb.py` 🚀
