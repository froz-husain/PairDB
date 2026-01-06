# pairDB Documentation

This directory contains the high-level design documentation for pairDB, a distributed key-value store.

## Document Structure

### 1. [Requirements Document](requirements.md)
Complete functional and non-functional requirements for the system.

### 2. [High-Level Design Document](high-level-design.md)
- Summary of requirements
- System architecture overview
- Component overviews (API Gateway, Coordinator, Storage Node, Metadata Store, Idempotency Store)
- Technology stack summary
- Key design decisions
- Simplified scalability considerations and edge case solutions

### 3. [API Contracts](api-contracts.md)
Complete API specifications including:
- Tenant Management APIs
- Key-Value APIs
- Request/response formats
- Error codes
- Consistency levels
- Examples

### 4. [Coordinator Design](CoordinatorNodeDesign/coordinator-design.md)
High-level design for the Coordinator service:
- Requirements
- Service architecture
- APIs offered (internal and external)
- Data stores used
- Key algorithms
- Performance optimizations
- Deployment considerations

### 5. [Storage Node Design](storageNodeDesign/storage-node-design.md)
High-level design for the Storage Node service:
- Requirements
- Service architecture
- APIs offered
- Storage architecture (commit log, cache, memtable, SSTables)
- Key operations
- Performance optimizations
- Deployment considerations

## Design Philosophy

This design is optimized for **decent scale** applications:
- Moderate to high QPS (thousands to tens of thousands per node)
- Terabyte-scale storage per cluster
- Simplified solutions for edge cases (practical over perfect)
- Focus on operational simplicity

## Next Steps

For low-level design, the following topics will be covered:
- Detailed sequence diagrams for all flows
- Implementation details for each component
- Database schema designs
- Protocol buffer definitions
- Error handling strategies
- Testing strategies
- Deployment configurations

## Key Technologies

- **API Gateway**: HTTP/REST (Kong, Envoy, or custom)
- **Coordinator**: HTTP/gRPC microservice (Go/Java/Python)
- **Storage Node**: Custom storage engine (Go/C++/Rust)
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

