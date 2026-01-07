// Package main provides the entry point for the API Gateway service.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/devrev/pairdb/api-gateway/internal/config"
	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"github.com/devrev/pairdb/api-gateway/internal/metrics"
	"github.com/devrev/pairdb/api-gateway/internal/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	// Initialize logger
	logger := initLogger()
	defer logger.Sync()

	logger.Info("starting API Gateway")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load configuration", zap.Error(err))
	}

	logger.Info("configuration loaded",
		zap.Int("server_port", cfg.Server.Port),
		zap.Strings("coordinator_endpoints", cfg.Coordinator.Endpoints),
	)

	// Initialize gRPC client
	grpcClient, err := grpc.NewClient(cfg.Coordinator, logger)
	if err != nil {
		logger.Fatal("failed to create gRPC client", zap.Error(err))
	}
	defer grpcClient.Close()

	logger.Info("gRPC client initialized")

	// Initialize metrics
	m := metrics.NewMetrics()
	m.SetHealthStatus(true)

	// Start metrics server if enabled
	var metricsServer *metrics.MetricsServer
	if cfg.Metrics.Enabled {
		metricsServer = metrics.NewMetricsServer(cfg.Metrics.Port, cfg.Metrics.Path, logger)
		go func() {
			if err := metricsServer.Start(); err != nil {
				logger.Error("metrics server error", zap.Error(err))
			}
		}()
		logger.Info("metrics server started",
			zap.Int("port", cfg.Metrics.Port),
			zap.String("path", cfg.Metrics.Path),
		)
	}

	// Initialize HTTP server
	httpServer := server.NewServer(cfg, grpcClient, logger)
	httpServer.SetupRoutes()

	// Start HTTP server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := httpServer.Start(); err != nil {
			errChan <- err
		}
	}()

	logger.Info("HTTP server started", zap.Int("port", cfg.Server.Port))

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("server error", zap.Error(err))
	}

	// Graceful shutdown
	logger.Info("initiating graceful shutdown")
	m.SetHealthStatus(false)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown HTTP server", zap.Error(err))
	}

	// Shutdown metrics server
	if metricsServer != nil {
		if err := metricsServer.Shutdown(ctx); err != nil {
			logger.Error("failed to shutdown metrics server", zap.Error(err))
		}
	}

	logger.Info("API Gateway shutdown complete")
}

// initLogger initializes the zap logger.
func initLogger() *zap.Logger {
	// Get log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Get log format from environment
	logFormat := os.Getenv("LOG_FORMAT")

	var config zap.Config
	if logFormat == "console" {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	config.Level = zap.NewAtomicLevelAt(level)
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	logger, err := config.Build()
	if err != nil {
		// Fallback to basic logger
		logger, _ = zap.NewProduction()
	}

	return logger
}
