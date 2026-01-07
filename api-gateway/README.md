# API Gateway

The API Gateway is the entry point for all client requests to pairDB. It is a stateless HTTP server that exposes REST APIs to clients and forwards requests to the Coordinator service via gRPC.

## Features

- **REST API**: Exposes RESTful endpoints for key-value operations and tenant management
- **gRPC Backend**: Communicates with Coordinator service via gRPC with connection pooling
- **Rate Limiting**: Configurable rate limiting to protect backend services
- **Health Checks**: Liveness and readiness probes for Kubernetes
- **Metrics**: Prometheus metrics for observability
- **Request Tracing**: Request ID propagation for distributed tracing
- **Graceful Shutdown**: Clean shutdown with in-flight request completion
- **Production Ready**: Docker, Kubernetes manifests, and monitoring included

## Quick Start

### Prerequisites

- Go 1.24+
- Coordinator service running (default: `localhost:50051`)

### Running Locally

```bash
# Build and run
make build
./bin/api-gateway --config config.yaml

# Or use make
make run
```

### Running with Docker

```bash
# Build Docker image
make docker

# Run container
docker run -p 8080:8080 -p 9090:9090 pairdb/api-gateway:latest
```

## API Endpoints

### Key-Value Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/key-value` | Write a key-value pair |
| GET | `/v1/key-value` | Read a key-value pair |

### Tenant Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/tenants` | Create a new tenant |
| GET | `/v1/tenants/{tenant_id}` | Get tenant information |
| PUT | `/v1/tenants/{tenant_id}/replication-factor` | Update replication factor |

### Admin Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/admin/storage-nodes` | Add a storage node |
| GET | `/v1/admin/storage-nodes` | List storage nodes |
| DELETE | `/v1/admin/storage-nodes/{node_id}` | Remove a storage node |
| GET | `/v1/admin/migrations/{migration_id}` | Get migration status |

### Health & Metrics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/ready` | Readiness probe |
| GET | `:9090/metrics` | Prometheus metrics |

## Example Usage

### Write a Key-Value Pair

```bash
curl -X POST http://localhost:8080/v1/key-value \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: unique-request-id" \
  -d '{
    "tenant_id": "my-tenant",
    "key": "user:123",
    "value": "{\"name\": \"John\", \"email\": \"john@example.com\"}",
    "consistency": "quorum"
  }'
```

### Read a Key-Value Pair

```bash
curl "http://localhost:8080/v1/key-value?tenant_id=my-tenant&key=user:123&consistency=quorum"
```

### Create a Tenant

```bash
curl -X POST http://localhost:8080/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "new-tenant",
    "replication_factor": 3
  }'
```

## Configuration

Configuration can be provided via:
1. Config file (`config.yaml`)
2. Environment variables (prefixed with `API_GATEWAY_`)

### Configuration Options

```yaml
server:
  port: 8080                    # HTTP server port
  read_timeout: 30s             # Read timeout
  write_timeout: 30s            # Write timeout
  idle_timeout: 120s            # Idle connection timeout
  shutdown_timeout: 30s         # Graceful shutdown timeout

coordinator:
  endpoints:
    - localhost:50051           # Coordinator gRPC endpoints
  timeout: 30s                  # gRPC call timeout
  max_retries: 3                # Max retry attempts
  retry_backoff: 100ms          # Initial retry backoff

rate_limiter:
  enabled: true                 # Enable rate limiting
  requests_per_second: 1000     # Requests per second limit
  burst_size: 100               # Burst allowance

metrics:
  enabled: true                 # Enable Prometheus metrics
  port: 9090                    # Metrics server port
  path: /metrics                # Metrics endpoint path

logging:
  level: info                   # Log level (debug, info, warn, error)
  format: json                  # Log format (json, console)
```

### Environment Variables

```bash
API_GATEWAY_SERVER_PORT=8080
API_GATEWAY_COORDINATOR_ENDPOINTS=coordinator:50051
API_GATEWAY_COORDINATOR_TIMEOUT=30s
API_GATEWAY_RATE_LIMITER_ENABLED=true
API_GATEWAY_METRICS_ENABLED=true
LOG_LEVEL=info
LOG_FORMAT=json
```

## Development

### Project Structure

```
api-gateway/
├── cmd/server/          # Application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── converter/       # HTTP ↔ gRPC conversion
│   ├── errors/          # Error handling
│   ├── grpc/            # gRPC client
│   ├── handler/         # HTTP handlers
│   ├── health/          # Health checks
│   ├── metrics/         # Prometheus metrics
│   ├── middleware/      # HTTP middleware
│   └── server/          # HTTP server
├── pkg/proto/           # Generated protobuf code
├── tests/
│   ├── unit/            # Unit tests
│   ├── integration/     # Integration tests
│   └── mocks/           # Test mocks
└── deployments/
    ├── docker/          # Dockerfile
    ├── k8s/             # Kubernetes manifests
    └── monitoring/      # Monitoring config
```

### Building

```bash
# Build binary
make build

# Build Docker image
make docker

# Clean build artifacts
make clean
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Generate coverage HTML report
make coverage-html

# Run linter
make lint
```

### Code Generation

```bash
# Generate protobuf files (requires protoc)
make proto
```

## Deployment

### Kubernetes

```bash
# Apply all manifests
kubectl apply -f deployments/k8s/

# Or apply individually
kubectl apply -f deployments/k8s/namespace.yaml
kubectl apply -f deployments/k8s/configmap.yaml
kubectl apply -f deployments/k8s/serviceaccount.yaml
kubectl apply -f deployments/k8s/deployment.yaml
kubectl apply -f deployments/k8s/service.yaml
kubectl apply -f deployments/k8s/hpa.yaml
kubectl apply -f deployments/k8s/pdb.yaml
kubectl apply -f deployments/k8s/ingress.yaml
```

### Resource Requirements

| Resource | Request | Limit |
|----------|---------|-------|
| CPU | 100m | 500m |
| Memory | 128Mi | 512Mi |

### Scaling

The API Gateway is stateless and can be horizontally scaled. The HPA is configured to:
- Minimum replicas: 3
- Maximum replicas: 10
- Scale on CPU > 70% or Memory > 80%

## Monitoring

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `api_gateway_http_requests_total` | Counter | Total HTTP requests |
| `api_gateway_http_request_duration_seconds` | Histogram | Request duration |
| `api_gateway_http_requests_in_flight` | Gauge | Current in-flight requests |
| `api_gateway_http_response_size_bytes` | Histogram | Response size |
| `api_gateway_grpc_requests_total` | Counter | gRPC requests to coordinator |
| `api_gateway_grpc_request_duration_seconds` | Histogram | gRPC request duration |
| `api_gateway_grpc_errors_total` | Counter | gRPC errors |
| `api_gateway_health_status` | Gauge | Health status (1=healthy, 0=unhealthy) |

### Alerts

See `deployments/monitoring/alerts.yaml` for Prometheus alerting rules.

## Error Handling

All errors follow a consistent format:

```json
{
  "status": "error",
  "error_code": "ERROR_CODE",
  "message": "Human-readable error message",
  "request_id": "uuid-of-request"
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Invalid request parameters |
| `TENANT_NOT_FOUND` | 404 | Tenant does not exist |
| `KEY_NOT_FOUND` | 404 | Key does not exist |
| `TENANT_EXISTS` | 409 | Tenant already exists |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `SERVICE_UNAVAILABLE` | 503 | Coordinator unavailable |
| `TIMEOUT` | 504 | Request timeout |
| `INTERNAL_ERROR` | 500 | Internal server error |

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Run `make verify` to ensure all checks pass
5. Submit a pull request

## License

Copyright 2024 DevRev. All rights reserved.

