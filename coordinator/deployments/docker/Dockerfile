# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protobuf-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /build/bin/coordinator \
    ./cmd/coordinator

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 coordinator && \
    adduser -D -u 1000 -G coordinator coordinator

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/coordinator /app/coordinator
COPY --from=builder /build/config.yaml /app/config.yaml

# Create necessary directories
RUN mkdir -p /app/logs && \
    chown -R coordinator:coordinator /app

# Switch to non-root user
USER coordinator

# Expose ports
EXPOSE 50051 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/health/live || exit 1

# Run the binary
ENTRYPOINT ["/app/coordinator"]
CMD ["--config", "/app/config.yaml"]
