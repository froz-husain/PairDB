# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protobuf-dev

# Disable Go workspace mode to avoid module resolution issues
ENV GOWORK=off

# Set working directory
WORKDIR /app

# Copy go mod files
COPY storage-node/go.mod storage-node/go.sum ./

# Copy coordinator dependency
COPY coordinator /coordinator

# Remove any go.work files that might have been copied
RUN rm -f go.work go.work.sum

# Download dependencies
RUN go mod download

# Copy storage-node source code
COPY storage-node .

# Generate protobuf code
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
RUN make proto

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o storage-node cmd/storage/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

# Create app user
RUN addgroup -g 1000 pairdb && \
    adduser -D -u 1000 -G pairdb pairdb

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/storage-node .
COPY --from=builder /app/config.yaml .

# Create data directories
RUN mkdir -p /var/lib/pairdb/commitlog /var/lib/pairdb/sstables && \
    chown -R pairdb:pairdb /var/lib/pairdb /app

# Switch to app user
USER pairdb

# Expose ports
EXPOSE 50052 7946 9091

# Set environment variable for config
ENV CONFIG_PATH=/app/config.yaml

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9091/health || exit 1

# Run the service
CMD ["./storage-node"]
