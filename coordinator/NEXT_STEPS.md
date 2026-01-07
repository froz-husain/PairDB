# Next Steps for Coordinator Implementation

## Current Status

The coordinator service foundation has been successfully created with the following components:

### Completed ✓
1. Project structure and Go module initialization
2. Proto definitions (coordinator.proto, storage.proto)
3. Domain models (tenant, vectorclock, hashring, migration)
4. Algorithm layer (consistent hashing, vector clock ops, quorum)
5. Store layer (metadata store, idempotency store, cache)
6. Client layer (storage client)
7. Configuration files (config.yaml)
8. Database migrations (001_initial_schema.sql)
9. Docker and Kubernetes manifests
10. Makefile for build automation
11. README documentation
12. Implementation status tracking

### Service Layer (Partial) ✓
- Vector clock service implemented

## Required Implementation Steps

### Step 1: Add Dependencies

Run the following command to add all required dependencies:

```bash
cd /Users/froz.husain/go/devrev.horizon.cloud/pairDB/coordinator

# Add all dependencies
go get google.golang.org/grpc@v1.58.0
go get google.golang.org/protobuf@v1.31.0
go get github.com/jackc/pgx/v5@v5.4.3
go get github.com/redis/go-redis/v9@v9.2.1
go get go.uber.org/zap@v1.26.0
go get github.com/prometheus/client_golang@v1.17.0
go get github.com/spf13/viper@v1.17.0
go get github.com/stretchr/testify@v1.8.4
go get github.com/google/uuid@v1.4.0
go get golang.org/x/sync@v0.4.0

# Tidy up
go mod tidy
```

### Step 2: Generate Proto Files

Install protoc and plugins, then generate:

```bash
# Install protoc-gen-go and protoc-gen-go-grpc
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate proto files
make proto
```

### Step 3: Implement Remaining Service Layer

Create these files in `internal/service/`:

#### 3.1 Consistency Service
File: `internal/service/consistency_service.go`

```go
package service

import "github.com/devrev/pairdb/coordinator/internal/algorithm"

type ConsistencyService struct {
    defaultLevel string
    quorum       *algorithm.QuorumCalculator
}

func NewConsistencyService(defaultLevel string) *ConsistencyService {
    return &ConsistencyService{
        defaultLevel: defaultLevel,
        quorum:       algorithm.NewQuorumCalculator(),
    }
}

func (s *ConsistencyService) GetRequiredReplicas(consistency string, totalReplicas int) int {
    return s.quorum.GetRequiredReplicas(consistency, totalReplicas)
}

func (s *ConsistencyService) ValidateConsistencyLevel(level string) bool {
    switch level {
    case "one", "quorum", "all":
        return true
    default:
        return false
    }
}
```

#### 3.2 Tenant Service
File: `internal/service/tenant_service.go`

Implementation provided in LLD lines 576-686. Key methods:
- `GetTenant(ctx, tenantID)` - with caching
- `CreateTenant(ctx, tenantID, replicationFactor)`
- `UpdateReplicationFactor(ctx, tenantID, newFactor)`

#### 3.3 Routing Service
File: `internal/service/routing_service.go`

Implementation provided in LLD lines 689-798. Key methods:
- `GetReplicas(tenantID, key, replicationFactor)`
- `updateHashRing(ctx)` - background refresh
- `refreshHashRing()` - periodic update

#### 3.4 Idempotency Service
File: `internal/service/idempotency_service.go`

Implementation provided in LLD lines 958-1027. Key methods:
- `Generate(tenantID, key)`
- `Get(ctx, tenantID, key, idempotencyKey)`
- `Store(ctx, tenantID, key, idempotencyKey, response)`

#### 3.5 Conflict Service
File: `internal/service/conflict_service.go`

Implementation provided in LLD lines 1029-1176. Key methods:
- `DetectConflicts(responses)`
- `TriggerRepair(ctx, tenantID, key, latest, replicas)`
- `repairWorker()` - background repair

#### 3.6 Coordinator Service (Main)
File: `internal/service/coordinator_service.go`

Implementation provided in LLD lines 313-574. Key methods:
- `WriteKeyValue(ctx, tenantID, key, value, consistency, idempotencyKey)`
- `ReadKeyValue(ctx, tenantID, key, consistency)`
- `writeToReplicas(ctx, replicas, ...)` - parallel writes
- `readFromReplicas(ctx, replicas, ...)` - parallel reads

### Step 4: Implement gRPC Handlers

Create these files in `internal/handler/`:

#### 4.1 Key-Value Handler
File: `internal/handler/keyvalue_handler.go`

Implementation provided in LLD lines 1770-1905. Implements:
- `WriteKeyValue(ctx, req)`
- `ReadKeyValue(ctx, req)`
- Request validation
- Proto conversion

#### 4.2 Tenant Handler
File: `internal/handler/tenant_handler.go`

Implements:
- `CreateTenant(ctx, req)`
- `UpdateReplicationFactor(ctx, req)`
- `GetTenant(ctx, req)`

#### 4.3 Node Handler
File: `internal/handler/node_handler.go`

Implements:
- `AddStorageNode(ctx, req)`
- `RemoveStorageNode(ctx, req)`
- `GetMigrationStatus(ctx, req)`
- `ListStorageNodes(ctx, req)`

### Step 5: Implement Configuration Management

File: `internal/config/config.go`

```go
package config

import (
    "time"
    "github.com/spf13/viper"
)

type Config struct {
    Server      ServerConfig
    Database    DatabaseConfig
    Redis       RedisConfig
    HashRing    HashRingConfig
    Consistency ConsistencyConfig
    Cache       CacheConfig
    Metrics     MetricsConfig
    Logging     LoggingConfig
}

// Load loads configuration from file and environment
func Load(configPath string) (*Config, error) {
    viper.SetConfigFile(configPath)
    viper.AutomaticEnv()

    if err := viper.ReadInConfig(); err != nil {
        return nil, err
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

Configuration structures provided in LLD lines 93-169.

### Step 6: Implement Metrics and Health Checks

#### 6.1 Prometheus Metrics
File: `internal/metrics/prometheus.go`

Implementation provided in LLD lines 2131-2195. Metrics:
- Request counters
- Latency histograms
- Cache hit/miss counters
- Conflict and repair counters
- Quorum failure counters

#### 6.2 Health Checks
File: `internal/health/health_check.go`

```go
package health

import (
    "context"
    "net/http"
    "go.uber.org/zap"
)

type HealthChecker struct {
    metadataStore    MetadataStorer
    idempotencyStore IdempotencyStorer
    logger           *zap.Logger
}

func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
    ctx := context.Background()

    // Check database
    if err := h.metadataStore.Ping(ctx); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }

    // Check Redis
    if err := h.idempotencyStore.Ping(ctx); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

### Step 7: Implement Server Setup

File: `internal/server/grpc_server.go`

```go
package server

import (
    "fmt"
    "net"
    "go.uber.org/zap"
    "google.golang.org/grpc"
)

type GRPCServer struct {
    server *grpc.Server
    logger *zap.Logger
}

func NewGRPCServer(logger *zap.Logger, opts ...grpc.ServerOption) *GRPCServer {
    return &GRPCServer{
        server: grpc.NewServer(opts...),
        logger: logger,
    }
}

func (s *GRPCServer) Start(host string, port int) error {
    addr := fmt.Sprintf("%s:%d", host, port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return err
    }

    s.logger.Info("Starting gRPC server", zap.String("address", addr))
    return s.server.Serve(listener)
}

func (s *GRPCServer) GracefulStop() {
    s.logger.Info("Shutting down gRPC server")
    s.server.GracefulStop()
}

func (s *GRPCServer) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
    s.server.RegisterService(desc, impl)
}
```

### Step 8: Implement Main Entry Point

File: `cmd/coordinator/main.go`

Structure provided in LLD lines 1910-2015. Key steps:
1. Initialize logger
2. Load configuration
3. Initialize stores (metadata, idempotency, cache)
4. Initialize services (all 7 services)
5. Initialize handlers (3 handlers)
6. Create and start gRPC server
7. Start metrics server
8. Handle graceful shutdown

### Step 9: Create Proto Generation Script

File: `scripts/gen_proto.sh`

```bash
#!/bin/bash

set -e

PROTO_DIR="api"
OUT_DIR="pkg/proto"

# Create output directory
mkdir -p $OUT_DIR

# Generate Go code
protoc --go_out=$OUT_DIR --go_opt=paths=source_relative \
    --go-grpc_out=$OUT_DIR --go-grpc_opt=paths=source_relative \
    $PROTO_DIR/*.proto

echo "Proto files generated successfully"
```

### Step 10: Write Tests

#### Unit Tests
- Test vector clock operations
- Test consistent hashing
- Test quorum calculation
- Test cache operations
- Test idempotency key generation

#### Integration Tests
- Test end-to-end write and read
- Test conflict detection
- Test repair mechanism
- Test tenant operations
- Test node addition/removal

### Step 11: Build and Run

```bash
# Generate proto files
make proto

# Download dependencies
make deps

# Build
make build

# Run database migrations
psql -d pairdb_metadata -f migrations/001_initial_schema.sql

# Run the service
./bin/coordinator --config config.yaml

# Or with Docker
make docker-build
make docker-run

# Or with Kubernetes
make k8s-deploy
```

## File Size Estimates

Based on the LLD, here are approximate sizes for remaining files:

| File | Lines | Complexity |
|------|-------|------------|
| coordinator_service.go | 300 | High |
| tenant_service.go | 150 | Medium |
| routing_service.go | 200 | High |
| consistency_service.go | 50 | Low |
| idempotency_service.go | 100 | Medium |
| conflict_service.go | 200 | High |
| keyvalue_handler.go | 200 | Medium |
| tenant_handler.go | 150 | Medium |
| node_handler.go | 200 | Medium |
| config.go | 150 | Medium |
| prometheus.go | 100 | Low |
| health_check.go | 100 | Low |
| grpc_server.go | 100 | Medium |
| main.go | 200 | High |
| **Total** | **~2,200 lines** | **-** |

## Implementation Priority

1. **Critical Path** (Required for MVP):
   - Configuration management
   - Remaining services (consistency, tenant, routing, idempotency, conflict, coordinator)
   - gRPC handlers (keyvalue, tenant, node)
   - Main entry point
   - Proto generation

2. **High Priority** (Required for production):
   - Metrics and health checks
   - Error handling improvements
   - Logging enhancements

3. **Medium Priority** (Important but not blocking):
   - Unit tests
   - Integration tests
   - Documentation improvements

4. **Low Priority** (Nice to have):
   - Benchmarks
   - Performance optimizations
   - Additional monitoring dashboards

## Estimated Time

For a senior engineer:
- Critical Path: 2-3 days
- High Priority: 1 day
- Medium Priority: 1-2 days
- Low Priority: 1 day

**Total: 5-7 days** for complete implementation and testing

## Support and References

- LLD document contains complete implementation details
- Each service has pseudo-code and structure defined
- Proto definitions are complete
- Algorithm implementations are tested patterns
- Configuration structure matches production requirements

## Questions or Issues?

If you encounter any issues during implementation:
1. Refer to the LLD for detailed specifications
2. Check the IMPLEMENTATION_STATUS.md for current status
3. Review the README.md for architecture overview
4. Examine existing implementations in the codebase
