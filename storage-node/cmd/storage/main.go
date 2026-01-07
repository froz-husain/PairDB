package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/devrev/pairdb/storage-node/internal/config"
	"github.com/devrev/pairdb/storage-node/internal/handler"
	"github.com/devrev/pairdb/storage-node/internal/service"
	pb "github.com/devrev/pairdb/storage-node/pkg/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./config.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("node_id", cfg.Server.NodeID),
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port))

	// Create data directories
	if err := os.MkdirAll(cfg.Storage.CommitLogDir, 0755); err != nil {
		logger.Fatal("Failed to create commit log directory", zap.Error(err))
	}
	if err := os.MkdirAll(cfg.Storage.SSTableDir, 0755); err != nil {
		logger.Fatal("Failed to create sstable directory", zap.Error(err))
	}

	// Initialize services
	commitLogSvc, err := service.NewCommitLogService(
		&service.CommitLogConfig{
			SegmentSize: cfg.CommitLog.SegmentSize,
			MaxAge:      cfg.CommitLog.MaxAge,
			SyncWrites:  cfg.CommitLog.SyncWrites,
			BufferSize:  cfg.CommitLog.BufferSize,
		},
		cfg.Storage.CommitLogDir,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to initialize commit log service", zap.Error(err))
	}
	defer commitLogSvc.Close()

	memTableSvc := service.NewMemTableService(
		&service.MemTableConfig{
			MaxSize:        cfg.MemTable.MaxSize,
			FlushThreshold: cfg.MemTable.FlushThreshold,
			NumMemTables:   cfg.MemTable.NumMemTables,
		},
		logger,
	)

	sstableSvc := service.NewSSTableService(
		&service.SSTableConfig{
			L0Size:          cfg.SSTable.L0Size,
			L1Size:          cfg.SSTable.L1Size,
			L2Size:          cfg.SSTable.L2Size,
			LevelMultiplier: cfg.SSTable.LevelMultiplier,
			BloomFilterFP:   cfg.SSTable.BloomFilterFP,
			BlockSize:       cfg.SSTable.BlockSize,
			IndexInterval:   cfg.SSTable.IndexInterval,
		},
		cfg.Storage.SSTableDir,
		logger,
	)

	cacheSvc := service.NewCacheService(
		&service.CacheConfig{
			MaxSize:         cfg.Cache.MaxSize,
			FrequencyWeight: cfg.Cache.FrequencyWeight,
			RecencyWeight:   cfg.Cache.RecencyWeight,
			AdaptiveWindow:  cfg.Cache.AdaptiveWindow,
		},
		logger,
	)

	vectorClockSvc := service.NewVectorClockService()

	compactionSvc := service.NewCompactionService(
		&service.CompactionConfig{
			L0Trigger: cfg.Compaction.L0Trigger,
			L0Size:    cfg.SSTable.L0Size,
			L1Size:    cfg.SSTable.L1Size,
			L2Size:    cfg.SSTable.L2Size,
			Workers:   cfg.Compaction.Workers,
			Throttle:  cfg.Compaction.Throttle,
			LevelMultiplier: cfg.SSTable.LevelMultiplier,
		},
		sstableSvc,
		logger,
	)
	defer compactionSvc.Stop()

	storageSvc := service.NewStorageService(
		commitLogSvc,
		memTableSvc,
		sstableSvc,
		cacheSvc,
		vectorClockSvc,
		logger,
		cfg.Server.NodeID,
	)

	// Recover from commit log
	logger.Info("Starting commit log recovery")
	if err := commitLogSvc.Recover(context.Background(), memTableSvc); err != nil {
		logger.Error("Failed to recover from commit log", zap.Error(err))
	}

	// Initialize gossip service if enabled
	if cfg.Gossip.Enabled {
		gossipSvc, err := service.NewGossipService(
			&service.GossipConfig{
				Enabled:        cfg.Gossip.Enabled,
				BindPort:       cfg.Gossip.BindPort,
				SeedNodes:      cfg.Gossip.SeedNodes,
				GossipInterval: cfg.Gossip.GossipInterval,
				ProbeTimeout:   cfg.Gossip.ProbeTimeout,
				ProbeInterval:  cfg.Gossip.ProbeInterval,
			},
			cfg.Server.NodeID,
			logger,
		)
		if err != nil {
			logger.Error("Failed to initialize gossip service", zap.Error(err))
		} else {
			defer gossipSvc.Shutdown()
			logger.Info("Gossip service initialized")
		}
	}

	// Initialize handlers
	storageHandler := handler.NewStorageHandler(storageSvc, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.MaxConcurrentStreams(uint32(cfg.Server.MaxConnections)),
	)

	pb.RegisterStorageNodeServiceServer(grpcServer, storageHandler)

	// Start listening
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	logger.Info("Storage node service starting",
		zap.String("node_id", cfg.Server.NodeID),
		zap.String("address", addr))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("Shutting down gracefully...")

		// Flush memtable
		if err := memTableSvc.Flush(context.Background(), sstableSvc); err != nil {
			logger.Error("Failed to flush memtable during shutdown", zap.Error(err))
		}

		grpcServer.GracefulStop()
	}()

	// Start server
	if err := grpcServer.Serve(listener); err != nil {
		logger.Fatal("Failed to serve", zap.Error(err))
	}
}

// initLogger initializes the zap logger
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
