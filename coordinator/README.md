# PairDB Coordinator Service

## Overview

The Coordinator service is a stateless microservice that handles request routing, consistency coordination, and conflict resolution for PairDB, a distributed key-value store. It acts as an intermediary between the API Gateway and Storage Nodes, managing the complexity of distributed operations.

## Architecture

### Key Components

1. **Request Routing**: Consistent hashing for replica selection
2. **Consistency Coordination**: Quorum-based read/write coordination
3. **Conflict Resolution**: Vector clock-based causality tracking
4. **Idempotency**: Redis-backed request deduplication
5. **Tenant Management**: PostgreSQL-backed configuration
6. **Caching**: In-memory tenant config cache

### Technology Stack

- **Language**: Go 1.21+
- **gRPC**: google.golang.org/grpc
- **Database**: PostgreSQL (pgx driver)
- **Cache**: Redis (go-redis)
- **Logging**: Uber Zap
- **Metrics**: Prometheus
- **Configuration**: Viper

## Write Flow with Vector Clocks

The coordinator implements a **read-modify-write pattern** for all write operations to maintain proper causality tracking with vector clocks.

### Write Operation Flow

```
1. Client Request
   ↓
2. Idempotency Check (Redis)
   ↓
3. Get Tenant Configuration (PostgreSQL/Cache)
   ↓
4. Resolve Replica Nodes (Consistent Hash Ring)
   ↓
5. Read Existing Value & Vector Clock ← CRITICAL STEP
   ↓
6. Generate New Vector Clock:
   - If key exists: IncrementFrom(existingVC)
   - If key not found: Increment() (new clock)
   ↓
7. Write to Replicas (Parallel)
   ↓
8. Check Quorum
   ↓
9. Store Idempotency Response
   ↓
10. Return Result
```

### Vector Clock Handling (Key Innovation)

**Problem Solved**: The original implementation created new vector clocks on every write, losing causality information and breaking conflict detection.

**Solution**: Read-modify-write pattern that preserves causality:

```go
// Step 1: Read existing value and vector clock
existingValue, existingVC, err := readExistingValue(ctx, replicas, tenantID, key)

// Step 2: Generate proper vector clock
if err != nil {
    // First write - create new clock
    vectorClock = vcService.Increment()
    // Result: VC={coord-1:1}
} else {
    // Update existing key - merge and increment
    vectorClock = vcService.IncrementFrom(existingVC)
    // Result: VC={coord-1:2} or VC={coord-1:5, coord-2:1}
}

// Step 3: Write with proper vector clock
writeToReplicas(ctx, replicas, tenantID, key, value, vectorClock, consistency)
```

### Causality Guarantees

**Sequential Writes (Same Coordinator)**:
```
Write 1: coord-1 → "Alice"  VC={coord-1:1}
Write 2: coord-1 → "Bob"    VC={coord-1:2}  ✓ Properly ordered
Write 3: coord-1 → "Carol"  VC={coord-1:3}  ✓ Causality chain maintained
```

**Concurrent Writes (Different Coordinators)**:
```
Initial: VC={coord-1:5}

Coord-1 writes "Alice":
  Read: VC={coord-1:5}
  Write: VC={coord-1:6}

Coord-2 writes "Bob" (concurrent):
  Read: VC={coord-1:5}
  Write: VC={coord-1:5, coord-2:1}  ✓ Detected as concurrent

Conflict Detection: Compare({coord-1:6}, {coord-1:5, coord-2:1})
Result: CONCURRENT → Read repair triggered with timestamp tiebreaker
```

**Multi-Coordinator Merging**:
```
State: VC={coord-1:10, coord-2:3}

Coord-3 writes:
  Read: VC={coord-1:10, coord-2:3}
  Write: VC={coord-1:10, coord-2:3, coord-3:1}  ✓ Full merge
```

### Performance Impact

- **Added Latency**: +1-5ms per write (ONE consistency read)
- **Optimization**: Uses fastest available replica
- **Trade-off**: Correctness > minimal latency increase

### Implementation Details

Key files:
- `internal/service/coordinator_service.go:129-158` - Read-modify-write logic
- `internal/service/coordinator_service.go:412-481` - readExistingValue helper
- `internal/service/vectorclock_service.go:43-76` - IncrementFrom method

## Project Structure

```
coordinator/
├── api/                      # Proto definitions
│   ├── coordinator.proto
│   └── storage.proto
├── cmd/coordinator/          # Main entry point
│   └── main.go
├── internal/
│   ├── algorithm/           # Core algorithms
│   │   ├── consistent_hash.go
│   │   ├── vectorclock_ops.go
│   │   └── quorum.go
│   ├── client/              # Storage node client
│   │   └── storage_client.go
│   ├── config/              # Configuration management
│   │   └── config.go
│   ├── handler/             # gRPC request handlers
│   │   ├── keyvalue_handler.go
│   │   ├── tenant_handler.go
│   │   └── node_handler.go
│   ├── health/              # Health checks
│   │   └── health_check.go
│   ├── metrics/             # Prometheus metrics
│   │   └── prometheus.go
│   ├── model/               # Domain models
│   │   ├── tenant.go
│   │   ├── vectorclock.go
│   │   ├── hashring.go
│   │   └── migration.go
│   ├── server/              # Server setup
│   │   └── grpc_server.go
│   ├── service/             # Business logic
│   │   ├── coordinator_service.go
│   │   ├── tenant_service.go
│   │   ├── routing_service.go
│   │   ├── consistency_service.go
│   │   ├── vectorclock_service.go
│   │   ├── idempotency_service.go
│   │   ├── conflict_service.go
│   │   └── migration_service.go
│   └── store/               # Data access layer
│       ├── metadata_store.go
│       ├── idempotency_store.go
│       └── cache.go
├── pkg/proto/               # Generated proto files
├── deployments/             # Deployment configs
│   ├── docker/
│   │   └── Dockerfile
│   └── k8s/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       └── hpa.yaml
├── migrations/              # Database migrations
│   └── 001_initial_schema.sql
├── scripts/                 # Build and utility scripts
│   └── gen_proto.sh
├── tests/                   # Tests
│   ├── unit/
│   └── integration/
├── config.yaml              # Default configuration
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 14+
- Redis 7+
- Protocol Buffers compiler (protoc)
- grpc Go plugins

### Installation

1. Clone the repository:
```bash
cd /Users/froz.husain/go/devrev.horizon.cloud/pairDB/coordinator
```

2. Install dependencies:
```bash
go mod download
```

3. Generate proto files:
```bash
make proto
```

4. Set up database:
```bash
# Create database
createdb pairdb_metadata

# Run migrations
psql -d pairdb_metadata -f migrations/001_initial_schema.sql
```

5. Configure environment:
```bash
cp config.yaml config.local.yaml
# Edit config.local.yaml with your settings
```

### Running the Service

```bash
# Development mode
go run cmd/coordinator/main.go --config config.local.yaml

# Build binary
make build

# Run binary
./bin/coordinator --config config.local.yaml
```

### Running with Docker

```bash
# Build image
docker build -t pairdb-coordinator:latest -f deployments/docker/Dockerfile .

# Run container
docker run -p 50051:50051 \
  -e DATABASE_HOST=postgres \
  -e REDIS_HOST=redis \
  pairdb-coordinator:latest
```

### Deploying to Kubernetes

```bash
# Apply manifests
kubectl apply -f deployments/k8s/

# Check status
kubectl get pods -l app=coordinator

# View logs
kubectl logs -l app=coordinator -f
```

## Configuration

### Configuration File (config.yaml)

```yaml
server:
  host: "0.0.0.0"
  port: 50051
  node_id: "coordinator-1"
  max_connections: 1000
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 30s

database:
  host: "localhost"
  port: 5432
  database: "pairdb_metadata"
  user: "coordinator"
  password: "secret"
  max_connections: 50
  min_connections: 10
  conn_max_lifetime: 30m

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
  max_retries: 3
  pool_size: 100
  min_idle_conns: 10

hash_ring:
  virtual_nodes: 150
  update_interval: 30s

consistency:
  default_level: "quorum"
  write_timeout: 5s
  read_timeout: 3s
  repair_async: true

cache:
  tenant_config_ttl: 5m
  max_size: 10000

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

logging:
  level: "info"
  format: "json"
```

### Environment Variables

Environment variables override config file settings:

- `COORDINATOR_NODE_ID`: Unique coordinator node ID
- `DATABASE_HOST`: PostgreSQL host
- `DATABASE_PORT`: PostgreSQL port
- `DATABASE_NAME`: Database name
- `DATABASE_USER`: Database user
- `DATABASE_PASSWORD`: Database password
- `REDIS_HOST`: Redis host
- `REDIS_PORT`: Redis port
- `REDIS_PASSWORD`: Redis password
- `LOG_LEVEL`: Log level (debug, info, warn, error)

## API Reference

### gRPC Service: CoordinatorService

#### WriteKeyValue

Writes a key-value pair to the distributed store.

**Request**:
```protobuf
message WriteKeyValueRequest {
  string tenant_id = 1;
  string key = 2;
  bytes value = 3;
  string consistency = 4;  // "one", "quorum", "all"
  string idempotency_key = 5;
}
```

**Response**:
```protobuf
message WriteKeyValueResponse {
  bool success = 1;
  string key = 2;
  string idempotency_key = 3;
  VectorClock vector_clock = 4;
  int32 replica_count = 5;
  string consistency = 6;
  bool is_duplicate = 7;
  string error_message = 8;
}
```

#### ReadKeyValue

Reads a key-value pair from the distributed store.

**Request**:
```protobuf
message ReadKeyValueRequest {
  string tenant_id = 1;
  string key = 2;
  string consistency = 3;
}
```

**Response**:
```protobuf
message ReadKeyValueResponse {
  bool success = 1;
  string key = 2;
  bytes value = 3;
  VectorClock vector_clock = 4;
  string error_message = 5;
}
```

See `api/coordinator.proto` for complete API documentation.

## Development

### Building

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

### Testing

```bash
# Run all tests
go test ./...

# Run unit tests only
go test ./tests/unit/...

# Run integration tests (requires PostgreSQL and Redis)
go test ./tests/integration/...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Code Style

- Follow standard Go code conventions
- Use `gofmt` for formatting
- Use `golangci-lint` for linting
- Write unit tests for all business logic
- Document public APIs with godoc comments

## Operations

### Health Checks

- **Liveness**: `GET /health/live`
- **Readiness**: `GET /health/ready`

### Metrics

Prometheus metrics exposed on `:9090/metrics`:

- `coordinator_requests_total`: Total number of requests
- `coordinator_request_duration_seconds`: Request latency histogram
- `coordinator_cache_hits_total`: Cache hit counter
- `coordinator_cache_misses_total`: Cache miss counter
- `coordinator_conflicts_detected_total`: Conflict detection counter
- `coordinator_repairs_triggered_total`: Repair operation counter
- `coordinator_quorum_failures_total`: Quorum failure counter

### Logging

Structured JSON logging with Zap:

```json
{
  "level": "info",
  "timestamp": "2024-01-07T10:30:00Z",
  "msg": "Request processed",
  "tenant_id": "tenant-123",
  "key": "user:456",
  "operation": "write",
  "duration_ms": 45,
  "success": true
}
```

### Troubleshooting

#### Common Issues

1. **Connection refused to storage nodes**
   - Check storage node health
   - Verify network connectivity
   - Check firewall rules

2. **Quorum not reached**
   - Check replication factor configuration
   - Verify storage node availability
   - Check network latency

3. **High latency**
   - Check database connection pool settings
   - Monitor Redis performance
   - Review cache hit rates
   - Check storage node performance

4. **Cache misses**
   - Increase cache TTL
   - Increase cache size
   - Check cache eviction patterns

## Performance

### Target Metrics

- **Latency**: p95 < 50ms for writes, p95 < 30ms for reads
- **Throughput**: 10,000+ QPS per coordinator instance
- **Availability**: 99.9% uptime
- **Cache Hit Rate**: > 90% for tenant configurations

### Tuning

1. **Connection Pools**:
   - PostgreSQL: 50-100 connections per coordinator
   - Redis: 100-200 connections per coordinator
   - Storage nodes: Connection per node

2. **Cache Settings**:
   - Tenant config TTL: 5 minutes
   - Max cache size: 10,000 entries

3. **Timeouts**:
   - Write timeout: 5 seconds
   - Read timeout: 3 seconds
   - Connection timeout: 5 seconds

4. **Hash Ring**:
   - Virtual nodes: 150 per physical node
   - Update interval: 30 seconds

## Monitoring

### Key Metrics to Monitor

1. **Request Metrics**:
   - Request rate (QPS)
   - Latency percentiles (p50, p95, p99)
   - Error rate

2. **Resource Metrics**:
   - CPU utilization
   - Memory usage
   - Network I/O

3. **Cache Metrics**:
   - Hit rate
   - Miss rate
   - Eviction rate

4. **Database Metrics**:
   - Connection pool usage
   - Query latency
   - Error rate

### Alerts

Set up alerts for:
- Error rate > 1%
- p95 latency > 100ms
- CPU utilization > 80%
- Cache hit rate < 80%
- Quorum failure rate > 0.1%

## Security

### Authentication

- Service-to-service authentication via mTLS
- Tenant authentication via JWT tokens

### Authorization

- Tenant isolation enforced at coordinator level
- Admin operations require special permissions

### Network Security

- TLS for all external communication
- Network policies in Kubernetes
- Firewall rules for PostgreSQL and Redis

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Write tests
5. Run linter and tests
6. Submit a pull request

## License

Copyright DevRev 2024. All rights reserved.

## Documentation Archive

The `memory/` folder contains implementation notes and historical documentation:
- **[INDEX.md](memory/INDEX.md)** - Guide to all memory folder contents
- **[SUMMARY.md](memory/SUMMARY.md)** - Current implementation status and statistics
- **[CHANGELOG.md](memory/CHANGELOG.md)** - Version history and recent changes
- **[BUILD_SUCCESS.md](memory/BUILD_SUCCESS.md)** - Build verification and syntax fixes
- **[QUICKSTART.md](memory/QUICKSTART.md)** - Quick setup guide
- **[IMPLEMENTATION_STATUS_FINAL.md](memory/IMPLEMENTATION_STATUS_FINAL.md)** - 95% completion status
- Other implementation reports and project deliverables

Use the memory folder for understanding implementation history, tracking changes, and quick troubleshooting references.

## References

- [High-Level Design](../docs/coordinator/design.md)
- [Low-Level Design](../docs/coordinator/low-level-design.md)
- [API Contracts](../docs/coordinator/api-contracts.md)
- [PairDB Documentation](../docs/README.md)
