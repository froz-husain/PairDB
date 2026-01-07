# API Gateway Test Plan

## 1. Overview

This document outlines the comprehensive test plan for the API Gateway component of pairDB. The test plan ensures production-ready quality with a target of **80%+ code coverage**.

## 2. Test Categories

### 2.1 Unit Tests

Unit tests validate individual components in isolation using mocks for dependencies.

#### 2.1.1 Configuration Tests (`tests/unit/config_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestConfigLoad_Defaults` | Verifies default configuration values are set correctly | Config loading |
| `TestConfigLoad_FromEnvironment` | Verifies environment variable overrides | Environment config |
| `TestConfigValidate_ValidConfig` | Validates a correct configuration passes validation | Config validation |
| `TestConfigValidate_InvalidServerPort` | Validates rejection of invalid server port | Error handling |
| `TestConfigValidate_NoCoordinatorEndpoints` | Validates requirement for coordinator endpoints | Error handling |
| `TestConfigValidate_InvalidCoordinatorTimeout` | Validates timeout validation | Error handling |
| `TestConfigValidate_InvalidRateLimiter` | Validates rate limiter configuration | Error handling |
| `TestConfigValidate_InvalidMetricsPort` | Validates metrics port validation | Error handling |

#### 2.1.2 Converter Tests (`tests/unit/converter_test.go`, `tests/unit/grpc_to_http_test.go`)

**HTTP to gRPC Conversion:**

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestHTTPToGRPC_WriteKeyValueRequest` | Tests valid and invalid write requests | Request parsing |
| `TestHTTPToGRPC_ReadKeyValueRequest` | Tests valid and invalid read requests | Query parameter parsing |
| `TestHTTPToGRPC_CreateTenantRequest` | Tests tenant creation request conversion | Body parsing |
| `TestHTTPToGRPC_UpdateReplicationFactorRequest` | Tests replication factor update | Path parameter extraction |
| `TestHTTPToGRPC_GetTenantRequest` | Tests tenant retrieval request | Path parameter validation |
| `TestHTTPToGRPC_AddStorageNodeRequest` | Tests storage node addition | Request validation |
| `TestHTTPToGRPC_RemoveStorageNodeRequest` | Tests storage node removal | Force parameter handling |
| `TestHTTPToGRPC_GetMigrationStatusRequest` | Tests migration status request | Path parameter extraction |
| `TestHTTPToGRPC_ListStorageNodesRequest` | Tests list storage nodes request | Empty request handling |

**gRPC to HTTP Conversion:**

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestGRPCToHTTP_WriteKeyValueResponse` | Tests write response conversion | Response formatting |
| `TestGRPCToHTTP_ReadKeyValueResponse` | Tests read response conversion | Value encoding |
| `TestGRPCToHTTP_CreateTenantResponse` | Tests tenant creation response | Timestamp handling |
| `TestGRPCToHTTP_UpdateReplicationFactorResponse` | Tests update response | Migration ID handling |
| `TestGRPCToHTTP_GetTenantResponse` | Tests tenant retrieval response | All fields mapping |
| `TestGRPCToHTTP_AddStorageNodeResponse` | Tests add node response | Estimated completion |
| `TestGRPCToHTTP_RemoveStorageNodeResponse` | Tests remove node response | Message formatting |
| `TestGRPCToHTTP_GetMigrationStatusResponse` | Tests migration status response | Progress handling |
| `TestGRPCToHTTP_ListStorageNodesResponse` | Tests list nodes response | Array conversion |

#### 2.1.3 Error Handler Tests (`tests/unit/error_handler_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestErrorHandler_GRPCToHTTPStatus` | Tests all gRPC to HTTP status code mappings | Status code mapping |
| `TestErrorHandler_GRPCToErrorCode` | Tests error code extraction from gRPC errors | Error code determination |
| `TestErrorHandler_WriteErrorResponse` | Tests error response formatting | Response writing |
| `TestErrorHandler_WriteValidationError` | Tests validation error responses | 400 Bad Request |
| `TestErrorHandler_WriteInternalError` | Tests internal error responses | 500 Internal Server Error |
| `TestErrorHandler_WriteServiceUnavailable` | Tests service unavailable responses | 503 Service Unavailable |
| `TestErrorHandler_WriteRateLimitedError` | Tests rate limit responses | 429 Too Many Requests |
| `TestErrorHandler_HandleError` | Tests full error handling flow | End-to-end error handling |

#### 2.1.4 Middleware Tests (`tests/unit/middleware_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestRequestIDMiddleware` | Tests request ID generation and preservation | Request tracking |
| `TestLoggingMiddleware` | Tests request logging | Observability |
| `TestRecoveryMiddleware` | Tests panic recovery | Error resilience |
| `TestCORSMiddleware` | Tests CORS header handling | Cross-origin support |
| `TestRateLimiterMiddleware` | Tests rate limiting behavior | Rate limiting |
| `TestContentTypeMiddleware` | Tests content type setting | Header management |
| `TestTimeoutMiddleware` | Tests context timeout | Timeout handling |
| `TestChainMiddleware` | Tests middleware chaining | Middleware composition |

#### 2.1.5 Health Check Tests (`tests/unit/health_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestHealthCheck_LivenessHandler` | Tests liveness endpoint | Health check |
| `TestHealthCheck_SetReady` | Tests readiness status management | Readiness tracking |

#### 2.1.6 Metrics Tests (`tests/unit/metrics_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestMetrics_RecordHTTPRequest` | Tests HTTP request metrics | Request tracking |
| `TestMetrics_RecordResponseSize` | Tests response size recording | Size metrics |
| `TestMetrics_RequestsInFlight` | Tests in-flight request counting | Concurrency metrics |
| `TestMetrics_RecordGRPCRequest` | Tests gRPC request metrics | Backend metrics |
| `TestMetrics_RecordGRPCError` | Tests gRPC error recording | Error metrics |
| `TestMetrics_SetHealthStatus` | Tests health status metric | Health metric |
| `TestMetricsMiddleware` | Tests metrics middleware | Automatic recording |

### 2.2 Integration Tests

Integration tests validate end-to-end request flows with mocked gRPC backend.

#### 2.2.1 Server Integration Tests (`tests/integration/server_test.go`)

| Test Case | Description | Coverage Target |
|-----------|-------------|-----------------|
| `TestIntegration_WriteKeyValue` | Tests full write flow | Write endpoint |
| `TestIntegration_ReadKeyValue` | Tests full read flow | Read endpoint |
| `TestIntegration_CreateTenant` | Tests tenant creation flow | Tenant management |
| `TestIntegration_GetTenant` | Tests tenant retrieval flow | Tenant lookup |
| `TestIntegration_ListStorageNodes` | Tests storage node listing | Admin endpoints |

## 3. Test Execution

### 3.1 Running All Tests

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...
```

### 3.2 Running Tests with Coverage

```bash
# Run tests with coverage report
make test-coverage

# Generate HTML coverage report
make coverage-html
```

### 3.3 Running Specific Test Categories

```bash
# Run unit tests only
go test -v ./tests/unit/...

# Run integration tests only
go test -v ./tests/integration/...

# Run specific test file
go test -v ./tests/unit/config_test.go

# Run specific test case
go test -v -run TestConfigLoad_Defaults ./tests/unit/...
```

## 4. Coverage Requirements

### 4.1 Minimum Coverage Targets

| Package | Minimum Coverage |
|---------|-----------------|
| `internal/config` | 90% |
| `internal/converter` | 90% |
| `internal/errors` | 85% |
| `internal/handler` | 80% |
| `internal/middleware` | 85% |
| `internal/health` | 80% |
| `internal/metrics` | 75% |
| `internal/grpc` | 70% |
| **Overall** | **80%** |

### 4.2 Coverage Exclusions

The following are excluded from coverage calculations:
- Generated protobuf code (`pkg/proto/`)
- Main entry point (`cmd/server/main.go`)
- Test files (`tests/`)

## 5. Mock Strategy

### 5.1 Mock Coordinator Client

The `MockCoordinatorServiceClient` in `tests/mocks/` provides:
- Configurable responses for all gRPC methods
- Error injection for failure testing
- Call verification with assertions

### 5.2 Mock Usage Example

```go
mockClient := mocks.NewMockCoordinatorServiceClient()
mockClient.On("WriteKeyValue", mock.Anything, mock.MatchedBy(func(req *pb.WriteKeyValueRequest) bool {
    return req.Key == "expected-key"
})).Return(&pb.WriteKeyValueResponse{
    Success: true,
    Key:     "expected-key",
}, nil)
```

## 6. Continuous Integration

### 6.1 CI Pipeline Steps

1. **Lint**: Run `golangci-lint` for code quality
2. **Test**: Run all tests with race detection
3. **Coverage**: Generate coverage report
4. **Coverage Gate**: Fail if coverage < 80%

### 6.2 Pre-commit Checks

```bash
# Run before committing
make verify
```

## 7. Test Data

### 7.1 Test Fixtures

Standard test data used across tests:
- Tenant ID: `"tenant1"`, `"tenant2"`, `"new-tenant"`
- Key names: `"mykey"`, `"test-key"`, `"nonexistent"`
- Consistency levels: `"one"`, `"quorum"`, `"all"`
- Node IDs: `"node1"`, `"node2"`

### 7.2 Error Scenarios

Common error scenarios tested:
- Missing required fields
- Invalid consistency levels
- Invalid port numbers
- Invalid replication factors
- Non-existent resources

## 8. Performance Testing

### 8.1 Benchmark Tests

```bash
# Run benchmarks
go test -bench=. -benchmem ./...
```

### 8.2 Load Testing

Recommended tools:
- `hey` - HTTP load testing
- `wrk` - HTTP benchmarking
- `ghz` - gRPC benchmarking (for end-to-end tests)

Example load test:
```bash
hey -n 10000 -c 100 http://localhost:8080/v1/key-value?tenant_id=t1&key=k1
```

## 9. Regression Testing

### 9.1 Key Regression Scenarios

1. **Request ID propagation** - Ensure request IDs flow through all layers
2. **Error format consistency** - All errors follow the standard format
3. **gRPC timeout handling** - Timeouts are properly converted to HTTP 504
4. **Rate limiting behavior** - Rate limits apply correctly under load

### 9.2 Regression Test Maintenance

- Add regression tests for any bug fixes
- Review test coverage after each release
- Update mocks when protobuf changes

## 10. Test Documentation

Each test file includes:
- Package-level documentation
- Test function documentation
- Table-driven test descriptions
- Expected behavior comments

## 11. Troubleshooting

### 11.1 Common Test Issues

| Issue | Solution |
|-------|----------|
| Mock not matching | Check `mock.MatchedBy` conditions |
| Timeout in tests | Increase context timeout |
| Race condition | Use `go test -race` |
| Coverage not updating | Clean test cache: `go clean -testcache` |

### 11.2 Debugging Tests

```bash
# Run with debug output
go test -v -run TestName ./tests/unit/ 2>&1 | tee test.log

# Use delve for debugging
dlv test ./tests/unit/ -- -test.run TestName
```

