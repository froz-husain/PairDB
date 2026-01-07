# PairDB - Distributed Key-Value Store

pairDB is a distributed, horizontally scalable key-value store designed for high availability and low latency at decent scale. It supports tunable consistency levels, multi-tenancy with per-tenant replication factors, and idempotency for write operations.

## Overview

pairDB is a production-ready distributed key-value store that provides:

- **Distributed Architecture**: Three-layer architecture with API Gateway, Coordinator, and Storage Nodes
- **Tunable Consistency**: Support for multiple consistency levels (one, quorum, all) at SDK or per-request level
- **Multi-Tenancy**: Complete tenant isolation with per-tenant replication factor configuration
- **High Availability**: Quorum-based replication ensures system remains operational with minority node failures
- **Conflict Resolution**: Vector clock-based causality tracking for conflict detection and resolution
- **Idempotency**: Client-provided or server-generated idempotency keys for exactly-once write semantics
- **Horizontal Scalability**: Dynamic node addition/removal with automatic data migration
- **Performance Optimized**: Optimized for small key-value pairs with low latency responses

## Architecture

pairDB consists of three main layers:

1. **API Gateway Layer**: Entry point for all client requests, handles authentication, rate limiting, and load balancing
2. **Coordinator Layer**: Stateless servers that handle request routing, consistency coordination, and conflict resolution
3. **Storage Layer**: Stateful nodes that store key-value data with multi-layer storage (commit log, cache, memtable, SSTables)

### Component Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          CLIENT LAYER                                    │
│                    ┌──────────────────────┐                             │
│                    │  Client Application  │                             │
│                    └──────────┬───────────┘                             │
│                               │ HTTP/REST                                │
│                               ▼                                          │
└───────────────────────────────┼──────────────────────────────────────────┘
                                │
┌───────────────────────────────┼──────────────────────────────────────────┐
│                    API GATEWAY LAYER                                      │
│                    ┌──────────────────────┐                              │
│                    │    API Gateway       │                              │
│                    │  - Authentication    │                              │
│                    │  - Rate Limiting    │                              │
│                    │  - Load Balancing   │                              │
│                    └──────────┬───────────┘                              │
│                               │ Route Request                             │
│                               ▼                                          │
└───────────────────────────────┼──────────────────────────────────────────┘
                                │
┌───────────────────────────────┼──────────────────────────────────────────┐
│                    COORDINATOR LAYER                                      │
│                    ┌──────────────────────┐                              │
│                    │    Coordinator      │                              │
│                    │  Stateless Service  │                              │
│                    │    HTTP/gRPC        │                              │
│                    └───────┬──────────────┘                              │
│                            │                                             │
│        ┌───────────────────┼───────────────────┐                        │
│        │                   │                   │                        │
│        ▼                   ▼                   ▼                        │
│  ┌──────────┐      ┌──────────────┐    ┌──────────────┐                 │
│  │  Redis   │◄─────┤  PostgreSQL  │    │    Redis     │                 │
│  │  Cache   │      │  (Metadata) │    │ (Idempotency)│                 │
│  │(Tenant   │      │              │    │              │                 │
│  │ Config)  │      └──────────────┘    └──────────────┘                 │
│  └────┬─────┘                                                           │
│       │                                                                  │
│       └──────────────────────────────────────────────────┐               │
│                                                          │               │
│                    ┌──────────────────────┐             │               │
│                    │  Consistent Hash     │             │               │
│                    │       Ring            │             │               │
│                    └──────────┬───────────┘             │               │
│                               │ Hash Key                 │               │
│                               ▼                          │               │
└───────────────────────────────┼──────────────────────────┼───────────────┘
                                │                          │
┌───────────────────────────────┼──────────────────────────┼───────────────┐
│                    STORAGE LAYER                          │               │
│                               │                          │               │
│        ┌───────────────────────┼──────────────────────┐  │               │
│        │                       │                      │  │               │
│        ▼                       ▼                      ▼  │               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │ Storage Node │    │ Storage Node │    │ Storage Node │              │
│  │      1       │    │      2       │    │      3       │              │
│  │              │    │              │    │              │              │
│  │ - Commit Log │    │ - Commit Log │    │ - Commit Log │              │
│  │ - Cache      │    │ - Cache      │    │ - Cache      │              │
│  │ - MemTable   │    │ - MemTable   │    │ - MemTable   │              │
│  │ - SSTables   │    │ - SSTables   │    │ - SSTables   │              │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘              │
│         │                   │                   │                      │
│         └───────────────────┼───────────────────┘                      │
│                             │ Response                                 │
│                             ▼                                          │
└─────────────────────────────┼──────────────────────────────────────────┘
                               │
                               │ (Back to Coordinator → Gateway → Client)
```

## Key Features

### 1. Tunable Consistency
- **One**: Read/write from/to a single replica (lowest latency, eventual consistency)
- **Quorum**: Read/write from/to majority of replicas (balanced consistency and availability)
- **All**: Read/write from/to all replicas (strongest consistency, highest latency)

### 2. Multi-Tenancy
- Complete tenant isolation with separate key-value namespaces
- Per-tenant replication factor configuration
- Independent scaling and configuration per tenant

### 3. Vector Clock-Based Conflict Resolution
- Tracks causality without requiring global ordering
- Automatic conflict detection and resolution
- Last-write-wins strategy with timestamp-based resolution

### 4. Idempotency
- Client-provided or server-generated idempotency keys
- Prevents duplicate writes from retries or network issues
- Exactly-once write semantics

### 5. Consistent Hashing
- Efficient data distribution across storage nodes
- Minimal rebalancing on node addition/removal
- Virtual nodes for better load distribution

### 6. Multi-Layer Storage
- **Commit Log**: Write-ahead logging for durability
- **In-Memory Cache**: Adaptive cache (LRU + LFU) for frequently accessed keys
- **MemTable**: In-memory sorted table for recent writes
- **SSTables**: Immutable sorted string tables on disk with level-based compaction

### 7. Horizontal Scalability
- Dynamic node addition/removal with automatic data migration
- Stateless coordinators for easy horizontal scaling
- Automatic scaling with Kubernetes HPA

## Technology Stack

- **API Gateway**: HTTP/REST server (Go)
- **Coordinator**: gRPC microservice (Go)
- **Storage Node**: Custom storage engine with LSM-tree (Go)
- **Metadata Store**: PostgreSQL (primary) + Redis (cache)
- **Idempotency Store**: Redis (distributed cache with TTL)
- **Container Orchestration**: Kubernetes
- **Monitoring**: Prometheus + Grafana

## Version Tag

The project uses **version tag `v1`** for Kubernetes deployments. Docker images are tagged with:
- `latest` for development
- Git tags (e.g., `v1.0.0`) for releases

## Quick Start

### Local Development Setup

For local development with Minikube, see the [Local Setup Guide](local_setup/README.md).

### Prerequisites

- Go 1.21+
- Kubernetes cluster (Minikube, Kind, or cloud provider)
- PostgreSQL 14+
- Redis 7+
- Protocol Buffers compiler (protoc)

### Building Components

```bash
# Build API Gateway
cd api-gateway
make build

# Build Coordinator
cd coordinator
make build

# Build Storage Node
cd storage-node
make build
```

## Documentation

### Design Documents

- **[High-Level Design](docs/high-level-design.md)**: System architecture, component overview, and key design decisions
- **[Requirements](docs/requirements.md)**: Functional and non-functional requirements
- **[API Contracts](docs/api-contracts.md)**: Complete API specifications
- **[Use Case Diagram](docs/use-case-diagram.md)**: Global use case diagram showing all actors and use cases

### Component-Specific Documentation

#### API Gateway
- **[Design Document](docs/api-gateway/design.md)**: High-level design for API Gateway
- **[Low-Level Design](docs/api-gateway/low-level-design.md)**: Detailed implementation specifications
- **[Class Diagram](docs/api-gateway/class-diagram.md)**: Core entities and their relationships
- **[Sequence Diagrams](docs/api-gateway/sequence-diagrams.md)**: All flow diagrams (write, read, tenant management, etc.)
- **[API Contracts](docs/api-gateway/design.md#api-endpoints)**: REST API endpoints

#### Coordinator
- **[Design Document](docs/coordinator/design.md)**: High-level design for Coordinator service
- **[Low-Level Design](docs/coordinator/low-level-design.md)**: Detailed implementation specifications
- **[Class Diagram](docs/coordinator/class-diagram.md)**: Core entities and their relationships
- **[Sequence Diagrams](docs/coordinator/sequence-diagrams.md)**: All flow diagrams (write, read, conflict resolution, etc.)
- **[API Contracts](docs/coordinator/api-contracts.md)**: gRPC API specifications

#### Storage Node
- **[Design Document](docs/storage-node/design.md)**: High-level design for Storage Node
- **[Low-Level Design](docs/storage-node/low-level-design.md)**: Detailed implementation specifications
- **[Class Diagram](docs/storage-node/class-diagram.md)**: Core entities and their relationships
- **[Sequence Diagrams](docs/storage-node/sequence-diagrams.md)**: All flow diagrams (write, read, compaction, repair, etc.)
- **[API Contracts](docs/storage-node/api-contracts.md)**: gRPC API specifications

### Component READMEs

- **[API Gateway README](api-gateway/README.md)**: API Gateway service documentation
- **[Coordinator README](coordinator/README.md)**: Coordinator service documentation
- **[Storage Node README](storage-node/README.md)**: Storage Node service documentation

## Project Structure

```
pairDB/
├── api-gateway/          # API Gateway service
│   ├── cmd/             # Application entry point
│   ├── internal/        # Internal packages
│   ├── pkg/             # Public packages
│   ├── deployments/     # Deployment configurations
│   └── tests/           # Test suites
│
├── coordinator/          # Coordinator service
│   ├── cmd/             # Application entry point
│   ├── internal/        # Internal packages
│   ├── pkg/             # Public packages
│   ├── deployments/     # Deployment configurations
│   ├── migrations/      # Database migrations
│   └── tests/           # Test suites
│
├── storage-node/         # Storage Node service
│   ├── cmd/             # Application entry point
│   ├── internal/        # Internal packages
│   ├── pkg/             # Public packages
│   ├── deployments/     # Deployment configurations
│   └── tests/           # Test suites
│
├── docs/                 # Design documentation
│   ├── high-level-design.md
│   ├── requirements.md
│   ├── api-contracts.md
│   ├── api-gateway/     # API Gateway design docs
│   ├── coordinator/     # Coordinator design docs
│   └── storage-node/    # Storage Node design docs
│
└── local_setup/          # Local development setup
    ├── README.md        # Local setup guide
    ├── deploy.sh        # Deployment script
    └── k8s/             # Local Kubernetes manifests
```

## Key Design Decisions

1. **Consistent Hashing**: Enables efficient data distribution and minimal rebalancing
2. **Quorum-based Replication**: Balances consistency and availability
3. **Vector Clocks**: Enables conflict detection without requiring global ordering
4. **Multi-layer Storage**: Commit Log → Cache → MemTable → SSTables for optimal performance
5. **Stateless Coordinators**: Simplifies scaling and load distribution
6. **Per-Tenant Replication**: Allows tenants to choose their availability/consistency trade-offs
7. **Redis for Idempotency**: Fast, distributed cache with TTL support
8. **PostgreSQL for Metadata**: Reliable, ACID-compliant storage for tenant configurations

## Performance Characteristics

- **Write Latency**: p95 < 50ms
- **Read Latency**: p95 < 30ms
- **Throughput**: 10,000+ QPS per coordinator instance
- **Availability**: 99.9% uptime target
- **Cache Hit Rate**: > 90% for tenant configurations

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Run linter and tests
5. Submit a pull request

## License

Copyright DevRev 2024. All rights reserved.

## References

- [High-Level Design Document](docs/high-level-design.md)
- [Requirements Document](docs/requirements.md)
- [API Contracts](docs/api-contracts.md)
- [Local Setup Guide](local_setup/README.md)

