# Changelog

## [Unreleased] - 2026-01-08

### Added
- **Read-Modify-Write Pattern**: Implemented proper vector clock handling in write operations
  - Added `IncrementFrom()` method to VectorClockService for causality-preserving increments
  - Added `readExistingValue()` helper method to read existing vector clocks before writes
  - Updated `WriteKeyValue()` to use read-modify-write pattern

### Changed
- **Vector Clock Generation**: Write operations now read existing vector clocks before incrementing
  - Preserves causality across sequential writes
  - Properly handles concurrent writes from multiple coordinators
  - Enables correct conflict detection and resolution

### Fixed
- **Lost Causality**: Fixed issue where new vector clocks were created on every write, losing causal relationships
- **Concurrent Write Conflicts**: Fixed improper handling of concurrent writes from different coordinators
- **Vector Clock Merging**: Fixed missing merge of coordinator timestamps

### Documentation
- Updated README.md with detailed write flow and vector clock handling
- Added comprehensive examples of sequential and concurrent write scenarios
- Documented causality guarantees and performance impact

## [1.0.0] - 2026-01-08

### Added
- Initial coordinator service implementation
- gRPC service with 10 RPC methods
- Consistent hashing with virtual nodes
- Quorum-based consistency (ONE, QUORUM, ALL)
- PostgreSQL metadata store
- Redis idempotency handling
- In-memory tenant configuration cache
- Health check endpoints
- Prometheus metrics
- Complete configuration management
- Database migrations

### Technical Stack
- Go 1.24.0
- gRPC v1.78.0
- Protocol Buffers v1.36.11
- PostgreSQL with pgx v5.4.3
- Redis with go-redis v9.2.1
- Viper v1.17.0 for configuration
- Zap v1.26.0 for logging
- Prometheus client v1.17.0

### Build
- 30MB binary size
- 3,747 lines of code
- 60+ Go files across 11 packages
