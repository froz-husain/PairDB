package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/client"
	"github.com/devrev/pairdb/coordinator/internal/config"
	"github.com/devrev/pairdb/coordinator/internal/handler"
	"github.com/devrev/pairdb/coordinator/internal/health"
	"github.com/devrev/pairdb/coordinator/internal/metrics"
	"github.com/devrev/pairdb/coordinator/internal/service"
	"github.com/devrev/pairdb/coordinator/internal/store"
	pb "github.com/devrev/pairdb/coordinator/pkg/proto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	logger.Info("Starting PairDB Coordinator Service")

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("node_id", cfg.Server.NodeID),
		zap.Int("port", cfg.Server.Port),
		zap.String("database_host", cfg.Database.Host),
		zap.Int("database_port", cfg.Database.Port),
		zap.String("database_name", cfg.Database.Database),
		zap.String("database_user", cfg.Database.User))

	// Initialize metrics
	_ = metrics.NewMetrics()
	logger.Info("Metrics initialized")

	// Initialize metadata store (PostgreSQL)
	metadataStore, err := store.NewPostgresMetadataStore(
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.MaxConnections,
		cfg.Database.MinConnections,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to initialize metadata store", zap.Error(err))
	}
	logger.Info("Metadata store initialized")

	// Get the PostgreSQL connection pool for shared use
	pgMetadataStore, ok := metadataStore.(*store.PostgresMetadataStore)
	if !ok {
		logger.Fatal("Failed to cast metadata store to PostgresMetadataStore")
	}

	// Initialize idempotency store (Redis)
	idempotencyStore, err := store.NewRedisIdempotencyStore(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to initialize idempotency store", zap.Error(err))
	}
	logger.Info("Idempotency store initialized")

	// Initialize cache (in-memory for now)
	cache := store.NewInMemoryCache(cfg.Cache.MaxSize, logger)
	logger.Info("Cache initialized")

	// Initialize hint store (PostgreSQL) for hinted handoff
	hintStore := store.NewPostgresHintStore(pgMetadataStore.GetPool())
	logger.Info("Hint store initialized")

	// Initialize storage client
	storageClient := client.NewStorageClient(cfg.Consistency.WriteTimeout)
	logger.Info("Storage client initialized")

	// Initialize services
	logger.Info("Initializing services")

	vectorClockService := service.NewVectorClockService(cfg.Server.NodeID)
	consistencyService := service.NewConsistencyService(cfg.Consistency.DefaultLevel)
	tenantService := service.NewTenantService(metadataStore, cache, cfg.Cache.TenantConfigTTL, logger)
	routingService := service.NewRoutingService(metadataStore, cfg.HashRing.VirtualNodes, cfg.HashRing.UpdateInterval, logger)
	idempotencyService := service.NewIdempotencyService(idempotencyStore, 24*time.Hour, logger)
	conflictService := service.NewConflictService(storageClient, vectorClockService, 10, cfg.Consistency.RepairAsync, logger)

	// Initialize hinted handoff service (V2 - Cassandra pattern)
	hintedHandoffService := service.NewHintedHandoffService(
		storageClient,
		10000,          // max hints per node
		3*time.Hour,    // hint TTL
		10*time.Second, // replay interval
		logger,
	)
	hintedHandoffService.Start()

	// Initialize key range service for Phase 2 streaming
	keyRangeService := service.NewKeyRangeService(logger)

	// Initialize storage node client for Phase 2 streaming
	storageNodeClient := client.NewStorageNodeClient(30*time.Second, logger)

	// Initialize hint replay service for Phase 6 (hinted handoff from database)
	hintReplayService := service.NewHintReplayService(hintStore, storageNodeClient, logger)

	// Use V2 coordinator service (Cassandra/DynamoDB pattern - no migration checks, immediate ring updates)
	coordinatorService := service.NewCoordinatorServiceV2(
		tenantService,
		routingService,
		consistencyService,
		idempotencyService,
		conflictService,
		vectorClockService,
		hintedHandoffService,
		storageClient,
		cfg.Consistency.WriteTimeout,
		cfg.Consistency.ReadTimeout,
		logger,
	)

	logger.Info("All services initialized")

	// Initialize handlers
	kvHandler := handler.NewKeyValueHandler(coordinatorService, logger)
	tenantHandler := handler.NewTenantHandler(tenantService, logger)
	// Use V2 handler with Phase 2 streaming support (Cassandra pattern) and hint replay
	nodeHandler := handler.NewNodeHandlerV2(metadataStore, routingService, keyRangeService, storageNodeClient, hintReplayService, logger)

	logger.Info("Handlers initialized")

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(10*1024*1024), // 10MB
		grpc.MaxSendMsgSize(10*1024*1024), // 10MB
	)

	// Register services - note: all handlers implement the same interface
	// We need to create a unified handler or register each RPC separately
	pb.RegisterCoordinatorServiceServer(grpcServer, &unifiedHandler{
		kvHandler:     kvHandler,
		tenantHandler: tenantHandler,
		nodeHandler:   nodeHandler,
	})

	logger.Info("gRPC services registered")

	// Start metrics server
	if cfg.Metrics.Enabled {
		go func() {
			mux := http.NewServeMux()
			mux.Handle(cfg.Metrics.Path, promhttp.Handler())
			addr := fmt.Sprintf(":%d", cfg.Metrics.Port)
			logger.Info("Starting metrics server", zap.String("address", addr))
			if err := http.ListenAndServe(addr, mux); err != nil {
				logger.Error("Metrics server failed", zap.Error(err))
			}
		}()
	}

	// Start health check server
	healthChecker := health.NewHealthChecker(metadataStore, idempotencyStore, cache, logger)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health/live", healthChecker.LivenessHandler)
		mux.HandleFunc("/health/ready", healthChecker.ReadinessHandler)
		addr := ":8080"
		logger.Info("Starting health check server", zap.String("address", addr))
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Error("Health check server failed", zap.Error(err))
		}
	}()

	// Start hint cleanup background process (Phase 6: Hinted Handoff)
	// Cleans up hints older than 7 days to prevent unbounded growth
	hintTTL := 7 * 24 * time.Hour // 7 days
	ctx := context.Background()
	go hintReplayService.CleanupOldHints(ctx, hintTTL)
	logger.Info("Started hint cleanup background process",
		zap.Duration("ttl", hintTTL))

	// Start gRPC server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("Failed to create listener", zap.Error(err))
	}

	logger.Info("Starting gRPC server", zap.String("address", addr))

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- grpcServer.Serve(listener)
	}()

	// Wait for interrupt signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("Server error", zap.Error(err))
	case sig := <-sigChan:
		logger.Info("Received signal", zap.String("signal", sig.String()))
	}

	// Graceful shutdown
	logger.Info("Shutting down gracefully")

	// Stop gRPC server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("gRPC server stop timeout, forcing shutdown")
		grpcServer.Stop()
	}

	// Stop services
	routingService.Stop()
	conflictService.Stop()
	storageClient.Close()

	// Close stores
	metadataStore.Close()
	idempotencyStore.Close()

	logger.Info("Coordinator service stopped")
}

// unifiedHandler combines all handlers into a single implementation
type unifiedHandler struct {
	pb.UnimplementedCoordinatorServiceServer
	kvHandler     *handler.KeyValueHandler
	tenantHandler *handler.TenantHandler
	nodeHandler   *handler.NodeHandlerV2
}

func (h *unifiedHandler) WriteKeyValue(ctx context.Context, req *pb.WriteKeyValueRequest) (*pb.WriteKeyValueResponse, error) {
	return h.kvHandler.WriteKeyValue(ctx, req)
}

func (h *unifiedHandler) ReadKeyValue(ctx context.Context, req *pb.ReadKeyValueRequest) (*pb.ReadKeyValueResponse, error) {
	return h.kvHandler.ReadKeyValue(ctx, req)
}

func (h *unifiedHandler) CreateTenant(ctx context.Context, req *pb.CreateTenantRequest) (*pb.CreateTenantResponse, error) {
	return h.tenantHandler.CreateTenant(ctx, req)
}

func (h *unifiedHandler) UpdateReplicationFactor(ctx context.Context, req *pb.UpdateReplicationFactorRequest) (*pb.UpdateReplicationFactorResponse, error) {
	return h.tenantHandler.UpdateReplicationFactor(ctx, req)
}

func (h *unifiedHandler) GetTenant(ctx context.Context, req *pb.GetTenantRequest) (*pb.GetTenantResponse, error) {
	return h.tenantHandler.GetTenant(ctx, req)
}

func (h *unifiedHandler) AddStorageNode(ctx context.Context, req *pb.AddStorageNodeRequest) (*pb.AddStorageNodeResponse, error) {
	return h.nodeHandler.AddStorageNode(ctx, req)
}

func (h *unifiedHandler) RemoveStorageNode(ctx context.Context, req *pb.RemoveStorageNodeRequest) (*pb.RemoveStorageNodeResponse, error) {
	return h.nodeHandler.RemoveStorageNode(ctx, req)
}

func (h *unifiedHandler) GetMigrationStatus(ctx context.Context, req *pb.GetMigrationStatusRequest) (*pb.GetMigrationStatusResponse, error) {
	return h.nodeHandler.GetMigrationStatus(ctx, req)
}

func (h *unifiedHandler) ListStorageNodes(ctx context.Context, req *pb.ListStorageNodesRequest) (*pb.ListStorageNodesResponse, error) {
	return h.nodeHandler.ListStorageNodes(ctx, req)
}
