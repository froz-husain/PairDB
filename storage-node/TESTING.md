# Testing Guide

## Overview

This document describes the testing strategy and how to run tests for the PairDB Storage Node.

## Test Structure

```
tests/
├── unit/               # Unit tests for individual components
│   ├── service/        # Service layer tests
│   ├── storage/        # Storage layer tests
│   └── handler/        # Handler tests
├── integration/        # Integration tests
└── mocks/             # Mock implementations
```

## Running Tests

### Prerequisites

```bash
# Install dependencies
make deps

# Install test tools
go install gotest.tools/gotestsum@latest
```

### All Tests

Run all tests with coverage:
```bash
make test
```

### Unit Tests Only

```bash
make test-unit
```

### Integration Tests Only

```bash
make test-integration
```

### Race Detector

Run tests with race detector enabled:
```bash
make test-race
```

### Coverage Report

Generate HTML coverage report:
```bash
make test-coverage
open coverage.html
```

### Coverage Threshold Check

Enforce 80% coverage threshold:
```bash
make test-coverage-check
```

### Benchmarks

Run performance benchmarks:
```bash
make bench
```

Run specific benchmark:
```bash
go test -bench=BenchmarkStorageService_Write -benchmem ./tests/unit/service/
```

## Test Categories

### 1. Unit Tests

**Location**: `tests/unit/`

Unit tests verify individual components in isolation with mocked dependencies.

#### Storage Service Tests
- `tests/unit/service/storage_service_test.go`
- Tests: Write, Read, Repair operations
- Coverage: Write validation, cache behavior, error handling

#### SkipList Tests
- `tests/unit/storage/skiplist_test.go`
- Tests: Insert, Search, Delete, Iterator
- Coverage: Edge cases, concurrent access (skipped), empty list

#### Running Unit Tests
```bash
# All unit tests
go test -v ./tests/unit/...

# Specific component
go test -v ./tests/unit/storage/

# With coverage
go test -v -cover ./tests/unit/...
```

### 2. Integration Tests

**Location**: `tests/integration/`

Integration tests verify end-to-end flows with real components.

#### Planned Tests
- Write/Read flow: Write data, read back, verify correctness
- Recovery: Crash simulation, restart, verify data integrity
- Compaction: Fill memtable, trigger flush, verify SSTable creation

```bash
# Run integration tests
make test-integration
```

### 3. Benchmark Tests

Benchmark tests measure performance characteristics.

```bash
# Run all benchmarks
make bench

# Example output:
# BenchmarkStorageService_Write-8    10000   112345 ns/op   1024 B/op   15 allocs/op
# BenchmarkStorageService_Read-8     20000    56789 ns/op    512 B/op    8 allocs/op
```

## Writing Tests

### Unit Test Template

```go
package service_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyComponent(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:    "valid input",
            input:   "test",
            want:    "result",
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup

            // Execute

            // Assert
            if tt.wantErr {
                assert.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Benchmark Template

```go
func BenchmarkMyOperation(b *testing.B) {
    // Setup

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Operation to benchmark
    }
}
```

## Test Best Practices

### 1. Table-Driven Tests
Use table-driven tests for comprehensive coverage:
```go
tests := []struct {
    name string
    // test parameters
}{
    {"case1", ...},
    {"case2", ...},
}
```

### 2. Test Isolation
- Each test should be independent
- Use `t.TempDir()` for temporary directories
- Clean up resources with `defer`

### 3. Deterministic Tests
- Avoid flaky tests
- Use fixed seeds for random data in tests
- Mock time-dependent operations

### 4. Error Path Testing
Always test error conditions:
```go
func TestErrorHandling(t *testing.T) {
    // Test with invalid input
    // Test with nil pointers
    // Test with resource exhaustion
}
```

### 5. Assertions
Use appropriate assertion methods:
```go
assert.NoError(t, err)          // Error should not occur
assert.Error(t, err)            // Error should occur
assert.Equal(t, expected, actual)
assert.True(t, condition)
require.NoError(t, err)         // Fatal if error
```

## Coverage Guidelines

### Target Coverage
- Overall: >80%
- Critical paths: >95%
- Error handling: >90%

### Measuring Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in terminal
go tool cover -func=coverage.out

# View coverage in browser
go tool cover -html=coverage.out
```

### Coverage Interpretation

```
file.go:45.16,47.2    2   100.0%  # All lines covered
file.go:50.16,52.2    2    66.7%  # Partial coverage
file.go:55.16,57.2    2     0.0%  # No coverage
```

## Continuous Integration

### GitHub Actions (Example)

```yaml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: make test-coverage-check
      - run: make test-race
```

## Test Data

### Creating Test Data

```go
func setupTestData(t *testing.T) {
    // Use t.TempDir() for file-based tests
    tmpDir := t.TempDir()

    // Create test fixtures
}
```

### Cleaning Up

```go
func TestWithCleanup(t *testing.T) {
    resource := createResource()
    defer resource.Close()

    t.Cleanup(func() {
        // Additional cleanup
    })
}
```

## Troubleshooting

### Tests Timing Out

```bash
# Increase timeout
go test -timeout=10m ./...
```

### Verbose Output

```bash
# Show all test output
go test -v ./...

# Show only failures
go test ./...
```

### Running Specific Tests

```bash
# By name
go test -run TestStorageService_Write ./tests/unit/service/

# By regex
go test -run "TestStorage.*Write" ./...
```

### Debug Tests

```go
func TestWithDebug(t *testing.T) {
    if testing.Verbose() {
        // Additional debug output
        t.Logf("Debug info: %v", data)
    }
}
```

## Performance Testing

### Load Testing

```bash
# Run benchmarks with specific duration
go test -bench=. -benchtime=30s ./...

# Memory profiling
go test -bench=. -memprofile=mem.prof ./...
go tool pprof mem.prof
```

### Profiling

```bash
# CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof

# In pprof:
(pprof) top10
(pprof) list functionName
```

## Test Maintenance

### Regular Tasks

1. Run full test suite before commits
2. Update tests when changing interfaces
3. Review and update mocks
4. Maintain test data fixtures
5. Keep coverage above threshold

### When to Add Tests

- New features: Add tests before implementation (TDD)
- Bug fixes: Add regression test first
- Refactoring: Ensure existing tests pass
- Performance changes: Add benchmarks

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Go Test Comments Best Practices](https://go.dev/doc/effective_go#commentary)
