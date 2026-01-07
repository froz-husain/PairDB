# API Gateway: Low-Level Design

## 1. Overview

This document provides a very high-level low-level design for the API Gateway implementation in Go. It outlines the package structure, key components, and implementation approach.

## 2. Package Structure

```
api-gateway/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── server/
│   │   ├── http_server.go       # HTTP server setup and routing
│   │   └── handlers.go          # HTTP request handlers
│   ├── grpc/
│   │   ├── client.go            # gRPC client connection management
│   │   └── coordinator.go       # Coordinator service client wrapper
│   ├── converter/
│   │   ├── http_to_grpc.go      # HTTP request to gRPC request conversion
│   │   └── grpc_to_http.go      # gRPC response to HTTP response conversion
│   ├── errors/
│   │   └── error_handler.go     # Error handling and HTTP status code mapping
│   └── config/
│       └── config.go            # Configuration management
├── pkg/
│   └── health/
│       └── health.go             # Health check endpoints
├── proto/                        # Generated protobuf code (from coordinator)
│   └── coordinator/
│       └── coordinator.pb.go
├── go.mod
├── go.sum
└── Dockerfile
```

## 3. Core Components

### 3.1 HTTP Server (`internal/server/http_server.go`)

**Responsibilities**:
- Initialize HTTP server with routes
- Start HTTP server on configured port
- Handle graceful shutdown

**Key Functions**:
```go
type Server struct {
    router      *mux.Router
    grpcClient  *grpc.CoordinatorClient
    httpServer  *http.Server
}

func NewServer(cfg *config.Config, grpcClient *grpc.CoordinatorClient) *Server
func (s *Server) SetupRoutes()
func (s *Server) Start() error
func (s *Server) Shutdown(ctx context.Context) error
```

### 3.2 Request Handlers (`internal/server/handlers.go`)

**Responsibilities**:
- Handle HTTP requests for each endpoint
- Parse HTTP request (body, query params, headers)
- Call converter to transform to gRPC request
- Call gRPC client
- Convert gRPC response to HTTP response
- Handle errors

**Key Functions**:
```go
type Handlers struct {
    grpcClient *grpc.CoordinatorClient
    converter  *converter.Converter
    errorHandler *errors.ErrorHandler
}

func (h *Handlers) WriteKeyValue(w http.ResponseWriter, r *http.Request)
func (h *Handlers) ReadKeyValue(w http.ResponseWriter, r *http.Request)
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request)
func (h *Handlers) UpdateReplicationFactor(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request)
func (h *Handlers) AddStorageNode(w http.ResponseWriter, r *http.Request)
func (h *Handlers) RemoveStorageNode(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetMigrationStatus(w http.ResponseWriter, r *http.Request)
func (h *Handlers) ListStorageNodes(w http.ResponseWriter, r *http.Request)
```

### 3.3 gRPC Client (`internal/grpc/client.go`)

**Responsibilities**:
- Manage gRPC connections to Coordinator service
- Connection pooling
- Load balancing across coordinator instances
- Health checks for connections

**Key Functions**:
```go
type Client struct {
    conn        *grpc.ClientConn
    coordinator coordinatorpb.CoordinatorServiceClient
    endpoints   []string
    balancer    LoadBalancer
}

func NewClient(endpoints []string) (*Client, error)
func (c *Client) GetCoordinatorClient() coordinatorpb.CoordinatorServiceClient
func (c *Client) Close() error
func (c *Client) HealthCheck() error
```

### 3.4 Coordinator Client Wrapper (`internal/grpc/coordinator.go`)

**Responsibilities**:
- Wrap gRPC client calls with error handling
- Implement retry logic (if needed)
- Add request context and timeouts

**Key Functions**:
```go
type CoordinatorClient struct {
    client coordinatorpb.CoordinatorServiceClient
}

func (c *CoordinatorClient) WriteKeyValue(ctx context.Context, req *coordinatorpb.WriteKeyValueRequest) (*coordinatorpb.WriteKeyValueResponse, error)
func (c *CoordinatorClient) ReadKeyValue(ctx context.Context, req *coordinatorpb.ReadKeyValueRequest) (*coordinatorpb.ReadKeyValueResponse, error)
func (c *CoordinatorClient) CreateTenant(ctx context.Context, req *coordinatorpb.CreateTenantRequest) (*coordinatorpb.CreateTenantResponse, error)
// ... other methods
```

### 3.5 HTTP to gRPC Converter (`internal/converter/http_to_grpc.go`)

**Responsibilities**:
- Convert HTTP JSON request body to gRPC request protobuf
- Extract query parameters and headers
- Map HTTP request fields to gRPC request fields

**Key Functions**:
```go
type Converter struct{}

func (c *Converter) WriteKeyValueRequest(r *http.Request) (*coordinatorpb.WriteKeyValueRequest, error)
func (c *Converter) ReadKeyValueRequest(r *http.Request) (*coordinatorpb.ReadKeyValueRequest, error)
func (c *Converter) CreateTenantRequest(r *http.Request) (*coordinatorpb.CreateTenantRequest, error)
// ... other conversion methods
```

**Example Implementation**:
```go
func (c *Converter) WriteKeyValueRequest(r *http.Request) (*coordinatorpb.WriteKeyValueRequest, error) {
    var httpReq struct {
        Key         string `json:"key"`
        Value       string `json:"value"`
        Consistency string `json:"consistency"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&httpReq); err != nil {
        return nil, err
    }
    
    // Extract tenant_id from context (set by middleware or extracted from token)
    tenantID := r.Context().Value("tenant_id").(string)
    
    // Extract idempotency key from header
    idempotencyKey := r.Header.Get("Idempotency-Key")
    
    return &coordinatorpb.WriteKeyValueRequest{
        TenantId:       tenantID,
        Key:            httpReq.Key,
        Value:          []byte(httpReq.Value),
        Consistency:    httpReq.Consistency,
        IdempotencyKey: idempotencyKey,
    }, nil
}
```

### 3.6 gRPC to HTTP Converter (`internal/converter/grpc_to_http.go`)

**Responsibilities**:
- Convert gRPC response protobuf to HTTP JSON response
- Format response according to API contract
- Handle vector clock serialization

**Key Functions**:
```go
func (c *Converter) WriteKeyValueResponse(grpcResp *coordinatorpb.WriteKeyValueResponse) (interface{}, error)
func (c *Converter) ReadKeyValueResponse(grpcResp *coordinatorpb.ReadKeyValueResponse) (interface{}, error)
func (c *Converter) CreateTenantResponse(grpcResp *coordinatorpb.CreateTenantResponse) (interface{}, error)
// ... other conversion methods
```

**Example Implementation**:
```go
func (c *Converter) WriteKeyValueResponse(grpcResp *coordinatorpb.WriteKeyValueResponse) (interface{}, error) {
    return map[string]interface{}{
        "status":         "success",
        "key":            grpcResp.Key,
        "idempotency_key": grpcResp.IdempotencyKey,
        "vector_clock":   convertVectorClock(grpcResp.VectorClock),
        "replica_count":  grpcResp.ReplicaCount,
        "consistency":    grpcResp.Consistency,
        "is_duplicate":   grpcResp.IsDuplicate,
    }, nil
}
```

### 3.7 Error Handler (`internal/errors/error_handler.go`)

**Responsibilities**:
- Map gRPC errors to HTTP status codes
- Format error responses consistently
- Log errors

**Key Functions**:
```go
type ErrorHandler struct {
    logger *log.Logger
}

func (e *ErrorHandler) HandleError(w http.ResponseWriter, err error)
func (e *ErrorHandler) GrpcToHTTPStatus(grpcErr error) int
func (e *ErrorHandler) WriteErrorResponse(w http.ResponseWriter, statusCode int, errorCode string, message string)
```

**Error Mapping**:
```go
func (e *ErrorHandler) GrpcToHTTPStatus(grpcErr error) int {
    if grpcErr == nil {
        return http.StatusOK
    }
    
    st, ok := status.FromError(grpcErr)
    if !ok {
        return http.StatusInternalServerError
    }
    
    switch st.Code() {
    case codes.OK:
        return http.StatusOK
    case codes.InvalidArgument:
        return http.StatusBadRequest
    case codes.NotFound:
        return http.StatusNotFound
    case codes.AlreadyExists:
        return http.StatusConflict
    case codes.Unavailable:
        return http.StatusServiceUnavailable
    case codes.DeadlineExceeded:
        return http.StatusGatewayTimeout
    default:
        return http.StatusInternalServerError
    }
}
```

### 3.8 Configuration (`internal/config/config.go`)

**Responsibilities**:
- Load configuration from environment variables or config file
- Provide default values
- Validate configuration

**Key Structures**:
```go
type Config struct {
    Server      ServerConfig
    Coordinator CoordinatorConfig
}

type ServerConfig struct {
    Port         int
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    IdleTimeout time.Duration
}

type CoordinatorConfig struct {
    Endpoints   []string
    Timeout     time.Duration
    MaxRetries  int
}
```

## 4. Request Flow Implementation

### 4.1 Write Request Flow

```go
func (h *Handlers) WriteKeyValue(w http.ResponseWriter, r *http.Request) {
    // 1. Convert HTTP request to gRPC request
    grpcReq, err := h.converter.WriteKeyValueRequest(r)
    if err != nil {
        h.errorHandler.WriteErrorResponse(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }
    
    // 2. Create context with timeout
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    // 3. Call gRPC client
    grpcResp, err := h.grpcClient.WriteKeyValue(ctx, grpcReq)
    if err != nil {
        statusCode := h.errorHandler.GrpcToHTTPStatus(err)
        h.errorHandler.HandleError(w, err)
        return
    }
    
    // 4. Convert gRPC response to HTTP response
    httpResp, err := h.converter.WriteKeyValueResponse(grpcResp)
    if err != nil {
        h.errorHandler.WriteErrorResponse(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
        return
    }
    
    // 5. Write HTTP response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(httpResp)
}
```

## 5. Health Check Endpoints

### 5.1 Health Check (`pkg/health/health.go`)

```go
func HealthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}

func ReadinessHandler(grpcClient *grpc.CoordinatorClient) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := grpcClient.HealthCheck(); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]string{
                "status": "not ready",
                "error":  err.Error(),
            })
            return
        }
        
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "ready",
        })
    }
}
```

## 6. Main Application (`cmd/server/main.go`)

```go
func main() {
    // 1. Load configuration
    cfg := config.Load()
    
    // 2. Initialize gRPC client
    grpcClient, err := grpc.NewClient(cfg.Coordinator.Endpoints)
    if err != nil {
        log.Fatal("Failed to create gRPC client:", err)
    }
    defer grpcClient.Close()
    
    // 3. Initialize handlers
    handlers := server.NewHandlers(grpcClient)
    
    // 4. Initialize HTTP server
    httpServer := server.NewServer(cfg, handlers)
    httpServer.SetupRoutes()
    
    // 5. Start server
    if err := httpServer.Start(); err != nil {
        log.Fatal("Failed to start server:", err)
    }
    
    // 6. Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := httpServer.Shutdown(ctx); err != nil {
        log.Fatal("Failed to shutdown server:", err)
    }
}
```

## 7. Key Implementation Details

### 7.1 Request Context
- Extract tenant_id from request context (set by external auth middleware or from token)
- Add request ID for tracing
- Set timeouts for gRPC calls

### 7.2 Error Handling
- All errors are logged with request context
- gRPC errors are converted to appropriate HTTP status codes
- Error responses follow consistent format

### 7.3 Connection Management
- gRPC connections are pooled and reused
- Connections are health-checked periodically
- Failed connections are retried with backoff

### 7.4 Load Balancing
- Round-robin across coordinator instances
- Health checks exclude unhealthy instances
- Connection pooling per coordinator instance

## 8. Testing Strategy

### 8.1 Unit Tests
- Test HTTP to gRPC conversion
- Test gRPC to HTTP conversion
- Test error handling and status code mapping
- Test request parsing

### 8.2 Integration Tests
- Test end-to-end request flow with mock gRPC server
- Test error scenarios
- Test load balancing

## 9. Dependencies

```go
require (
    github.com/gorilla/mux v1.8.0
    google.golang.org/grpc v1.50.0
    google.golang.org/protobuf v1.28.1
)
```

## 10. Configuration Example

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

coordinator:
  endpoints:
    - coordinator-service:50051
  timeout: 30s
  max_retries: 3
```

## 11. Deployment Considerations

- Stateless design allows horizontal scaling
- Health check endpoints for Kubernetes probes
- Graceful shutdown for zero-downtime deployments
- Configuration via environment variables or ConfigMaps
- Logging to stdout/stderr for Kubernetes log aggregation

