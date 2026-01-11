# PairDB Documentation

This directory contains the complete design and operational documentation for pairDB, a production-ready distributed key-value store with Cassandra-correct topology management, hinted handoff for write durability, and comprehensive safety mechanisms.

## Document Structure

### 1. [Requirements Document](requirements.md)
Complete functional and non-functional requirements for the system.

### 2. [High-Level Design Document](high-level-design.md)
- Summary of requirements
- System architecture overview
- Component overviews (API Gateway, Coordinator, Storage Node, Metadata Store, Idempotency Store)
- Technology stack summary
- Key design decisions
- **Phase 2 Streaming**: Cassandra-pattern node addition/removal with zero downtime
  - Immediate ring integration for node addition
  - Hash-range based data transfer
  - Background monitoring with automatic rollback
  - Edge case handling (read staleness, quorum consistency, race conditions, hash ring update timing)
  - Complete flow specifications for node addition and removal
  - Node lifecycle management (`active`, `bootstrapping`, `draining`, `failed`)
- Simplified scalability considerations and edge case solutions

### 3. [Use Case Diagram](use-case-diagram.md)
Global use case diagram for pairDB showing:
- All actors (Client, Administrator)
- Use cases for each service layer
- Relationships between use cases
- System boundaries

### 4. [API Contracts](api-contracts.md)
Complete API specifications including:
- Tenant Management APIs
- Key-Value APIs
- Request/response formats
- Error codes
- Consistency levels
- Examples

### 5. [API Gateway Design](api-gateway/design.md)
High-level design for the API Gateway service:
- Requirements
- Service architecture
- REST API endpoints
- HTTP to gRPC conversion
- Load balancing
- Error handling
- Deployment considerations

### 5.1. [API Gateway Low-Level Design](api-gateway/low-level-design.md)
Very high-level low-level design for API Gateway implementation:
- Package structure
- Core components
- Request flow implementation
- Error handling
- Testing strategy

### 5.2. [API Gateway Class Diagram](api-gateway/class-diagram.md)
Class diagram showing:
- Core entities and their relationships
- Component interactions
- Data structures

### 5.3. [API Gateway Sequence Diagrams](api-gateway/sequence-diagrams.md)
Sequence diagrams for all flows:
- Write Key-Value flow
- Read Key-Value flow
- Create Tenant flow
- Update Replication Factor flow
- Add Storage Node flow
- Error handling flow
- Health check flow
- Idempotency key handling flow

### 6. [Coordinator Design](coordinator/design.md)
High-level design for the Coordinator service:
- Requirements and service architecture
- APIs offered (internal and external)
- Data stores used (PostgreSQL metadata, Redis idempotency, PostgreSQL hints)
- Key algorithms (consistent hashing, vector clocks, quorum)
- **Hinted Handoff System** (Phase 6): Write durability during node failures
- **Cleanup Safety System** (Phase 7): Grace periods and quorum verification
- **Concurrent Node Operations** (Phase 8): Per-node locking for scalability
- **Replica Selection**: Split GetWriteReplicas/GetReadReplicas with authoritative replica concept
- Storage node lifecycle management
- Performance optimizations and deployment considerations

### 6.1. [Coordinator Low-Level Design](coordinator/low-level-design.md)
Detailed implementation specifications for Coordinator service.

### 6.2. [Coordinator Class Diagram](coordinator/class-diagram.md)
Class diagram showing:
- Core entities and their relationships
- Service layer components
- Data models and algorithms

### 6.3. [Coordinator Sequence Diagrams](coordinator/sequence-diagrams.md)
Sequence diagrams for all flows:
- Write Key-Value flow (Quorum Consistency)
- Read Key-Value flow (Quorum Consistency)
- Conflict Resolution flow
- Create Tenant flow
- Update Replication Factor flow
- Add Storage Node flow
- Hash Ring Update flow
- Idempotency Check flow

### 7. [Storage Node Design](storage-node/design.md)
High-level design for the Storage Node service:
- Requirements
- Service architecture
- APIs offered
- Storage architecture (commit log, cache, memtable, SSTables)
- Key operations
- Performance optimizations
- Deployment considerations

### 7.1. [Storage Node Low-Level Design](storage-node/low-level-design.md)
Detailed implementation specifications for Storage Node service.

### 7.2. [Storage Node Class Diagram](storage-node/class-diagram.md)
Class diagram showing:
- Core entities and their relationships
- Storage layer components
- Data structures (MemTable, SSTable, Cache, etc.)

### 7.3. [Storage Node Sequence Diagrams](storage-node/sequence-diagrams.md)
Sequence diagrams for all flows:
- Write Operation flow
- Read Operation flow
- MemTable Flush flow
- Compaction flow
- Repair Operation flow
- Commit Log Recovery flow
- Cache Eviction flow
- Gossip Health Monitoring flow

### 7.4. [Storage Node Operational Procedures](storage-node/operational-procedures.md)
Operational procedures for managing storage nodes in production:
- Node bootstrap and decommission procedures
- Monitoring and health checks
- Troubleshooting common issues
- Maintenance tasks and best practices

## Design Philosophy

This design is optimized for **decent scale** applications:
- Moderate to high QPS (thousands to tens of thousands per node)
- Terabyte-scale storage per cluster
- Simplified solutions for edge cases (practical over perfect)
- Focus on operational simplicity

## Design Diagrams

The documentation includes comprehensive diagrams:

- **Use Case Diagram**: Global view of all system use cases
- **Class Diagrams**: Entity relationships for each service
- **Sequence Diagrams**: Detailed flow diagrams for all operations

These diagrams provide visual representation of the system architecture and help understand the interactions between components.

## Key Technologies

- **API Gateway**: HTTP custom
- **Coordinator**: gRPC microservice (Go)
- **Storage Node**: Custom storage engine (Go)
- **Metadata Store**: PostgreSQL (primary) + Redis (cache)
- **Idempotency Store**: Redis (distributed cache with TTL)

## Design Decisions Summary

1. **Consistent Hashing**: For efficient data distribution
2. **Quorum-based Replication**: Balance consistency and availability
3. **Vector Clocks**: Conflict detection without global ordering
4. **Multi-layer Storage**: Commit Log → Cache → MemTable → SSTables
5. **Stateless Coordinators**: Easy horizontal scaling
6. **Per-Tenant Replication**: Tenant-specific availability/consistency trade-offs
7. **Redis for Idempotency**: Fast, distributed cache with TTL
8. **PostgreSQL for Metadata**: Reliable, ACID-compliant storage

