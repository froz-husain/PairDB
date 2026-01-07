// Package server provides the HTTP server implementation for the API Gateway.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/devrev/pairdb/api-gateway/internal/config"
	apierrors "github.com/devrev/pairdb/api-gateway/internal/errors"
	"github.com/devrev/pairdb/api-gateway/internal/grpc"
	"github.com/devrev/pairdb/api-gateway/internal/handler"
	"github.com/devrev/pairdb/api-gateway/internal/health"
	"github.com/devrev/pairdb/api-gateway/internal/middleware"
	"go.uber.org/zap"
)

// Server represents the HTTP server.
type Server struct {
	router       *mux.Router
	httpServer   *http.Server
	grpcClient   *grpc.Client
	handlers     *handler.Handlers
	healthCheck  *health.HealthCheck
	errorHandler *apierrors.Handler
	logger       *zap.Logger
	cfg          *config.Config
}

// NewServer creates a new HTTP server.
func NewServer(cfg *config.Config, grpcClient *grpc.Client, logger *zap.Logger) *Server {
	router := mux.NewRouter()
	errorHandler := apierrors.NewHandler(logger)
	handlers := handler.NewHandlers(grpcClient, errorHandler, logger, cfg.Coordinator)
	healthCheck := health.NewHealthCheck(grpcClient, logger)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return &Server{
		router:       router,
		httpServer:   httpServer,
		grpcClient:   grpcClient,
		handlers:     handlers,
		healthCheck:  healthCheck,
		errorHandler: errorHandler,
		logger:       logger,
		cfg:          cfg,
	}
}

// SetupRoutes configures all HTTP routes.
func (s *Server) SetupRoutes() {
	// Setup middleware chain
	middlewareChain := []func(http.Handler) http.Handler{
		middleware.Recovery(s.logger),
		middleware.RequestID,
		middleware.Logging(s.logger),
		middleware.CORS([]string{"*"}),
	}

	// Add rate limiter if enabled
	if s.cfg.RateLimiter.Enabled {
		rateLimiter := middleware.NewRateLimiter(
			s.cfg.RateLimiter.RequestsPerSecond,
			s.cfg.RateLimiter.BurstSize,
			s.logger,
		)
		middlewareChain = append(middlewareChain, rateLimiter.Limit)
	}

	// Apply middleware to router
	chain := middleware.Chain(middlewareChain...)
	s.router.Use(func(next http.Handler) http.Handler {
		return chain(next)
	})

	// Health check endpoints
	s.router.HandleFunc("/health", s.healthCheck.LivenessHandler).Methods(http.MethodGet)
	s.router.HandleFunc("/ready", s.healthCheck.ReadinessHandler).Methods(http.MethodGet)

	// API v1 routes
	v1 := s.router.PathPrefix("/v1").Subrouter()

	// Key-Value operations
	v1.HandleFunc("/key-value", s.handlers.WriteKeyValue).Methods(http.MethodPost)
	v1.HandleFunc("/key-value", s.handlers.ReadKeyValue).Methods(http.MethodGet)

	// Tenant management
	v1.HandleFunc("/tenants", s.handlers.CreateTenant).Methods(http.MethodPost)
	v1.HandleFunc("/tenants/{tenant_id}", s.handlers.GetTenant).Methods(http.MethodGet)
	v1.HandleFunc("/tenants/{tenant_id}/replication-factor", s.handlers.UpdateReplicationFactor).Methods(http.MethodPut)

	// Admin routes for storage node management
	admin := v1.PathPrefix("/admin").Subrouter()
	admin.HandleFunc("/storage-nodes", s.handlers.AddStorageNode).Methods(http.MethodPost)
	admin.HandleFunc("/storage-nodes", s.handlers.ListStorageNodes).Methods(http.MethodGet)
	admin.HandleFunc("/storage-nodes/{node_id}", s.handlers.RemoveStorageNode).Methods(http.MethodDelete)
	admin.HandleFunc("/migrations/{migration_id}", s.handlers.GetMigrationStatus).Methods(http.MethodGet)

	// Not found handler
	s.router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		s.errorHandler.WriteErrorResponse(w, http.StatusNotFound, apierrors.ErrorCodeInvalidRequest, "endpoint not found", requestID)
	})

	// Method not allowed handler
	s.router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		s.errorHandler.WriteErrorResponse(w, http.StatusMethodNotAllowed, apierrors.ErrorCodeInvalidRequest, "method not allowed", requestID)
	})
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server",
		zap.Int("port", s.cfg.Server.Port),
	)
	
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// GetRouter returns the router for testing purposes.
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// GetHandler returns the http.Handler for the server.
func (s *Server) GetHandler() http.Handler {
	return s.router
}

// StartAsync starts the server in a goroutine.
func (s *Server) StartAsync() chan error {
	errChan := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errChan <- err
		}
		close(errChan)
	}()
	
	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)
	return errChan
}

