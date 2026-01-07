package service

import (
	"context"
	"fmt"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
	"github.com/devrev/pairdb/coordinator/internal/store"
	"go.uber.org/zap"
)

// TenantService manages tenant configurations
type TenantService struct {
	metadataStore store.MetadataStore
	cache         store.Cache
	cacheTTL      time.Duration
	logger        *zap.Logger
}

// NewTenantService creates a new tenant service
func NewTenantService(
	metadataStore store.MetadataStore,
	cache store.Cache,
	cacheTTL time.Duration,
	logger *zap.Logger,
) *TenantService {
	return &TenantService{
		metadataStore: metadataStore,
		cache:         cache,
		cacheTTL:      cacheTTL,
		logger:        logger,
	}
}

// GetTenant retrieves tenant configuration, using cache if available
func (s *TenantService) GetTenant(ctx context.Context, tenantID string) (*model.Tenant, error) {
	// Try cache first
	cacheKey := s.tenantCacheKey(tenantID)
	if cached, err := s.cache.Get(ctx, cacheKey); err == nil && cached != nil {
		if tenant, ok := cached.(*model.Tenant); ok {
			s.logger.Debug("Tenant config retrieved from cache",
				zap.String("tenant_id", tenantID))
			return tenant, nil
		}
	}

	// Cache miss, fetch from metadata store
	s.logger.Debug("Cache miss for tenant, fetching from database",
		zap.String("tenant_id", tenantID))

	tenant, err := s.metadataStore.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tenant from metadata store: %w", err)
	}

	// Store in cache
	if err := s.cache.Set(ctx, cacheKey, tenant, s.cacheTTL); err != nil {
		s.logger.Warn("Failed to cache tenant config",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
	}

	return tenant, nil
}

// CreateTenant creates a new tenant configuration
func (s *TenantService) CreateTenant(ctx context.Context, tenantID string, replicationFactor int) (*model.Tenant, error) {
	// Validate replication factor
	if replicationFactor < 1 || replicationFactor > 10 {
		return nil, fmt.Errorf("invalid replication factor: %d (must be between 1 and 10)", replicationFactor)
	}

	tenant := &model.Tenant{
		TenantID:          tenantID,
		ReplicationFactor: replicationFactor,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Version:           1,
	}

	// Save to metadata store
	if err := s.metadataStore.CreateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant in metadata store: %w", err)
	}

	s.logger.Info("Created tenant",
		zap.String("tenant_id", tenantID),
		zap.Int("replication_factor", replicationFactor))

	// Cache the new tenant config
	cacheKey := s.tenantCacheKey(tenantID)
	if err := s.cache.Set(ctx, cacheKey, tenant, s.cacheTTL); err != nil {
		s.logger.Warn("Failed to cache new tenant config",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
	}

	return tenant, nil
}

// UpdateReplicationFactor updates the replication factor for a tenant
func (s *TenantService) UpdateReplicationFactor(ctx context.Context, tenantID string, newFactor int) (*model.Tenant, error) {
	// Validate new replication factor
	if newFactor < 1 || newFactor > 10 {
		return nil, fmt.Errorf("invalid replication factor: %d (must be between 1 and 10)", newFactor)
	}

	// Get current tenant config
	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	oldFactor := tenant.ReplicationFactor

	// Check if update is needed
	if oldFactor == newFactor {
		s.logger.Info("Replication factor unchanged",
			zap.String("tenant_id", tenantID),
			zap.Int("replication_factor", newFactor))
		return tenant, nil
	}

	// Update tenant
	tenant.ReplicationFactor = newFactor
	tenant.UpdatedAt = time.Now()
	tenant.Version++

	// Save to metadata store
	if err := s.metadataStore.UpdateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to update tenant in metadata store: %w", err)
	}

	s.logger.Info("Updated replication factor",
		zap.String("tenant_id", tenantID),
		zap.Int("old_factor", oldFactor),
		zap.Int("new_factor", newFactor))

	// Invalidate cache
	cacheKey := s.tenantCacheKey(tenantID)
	if err := s.cache.Delete(ctx, cacheKey); err != nil {
		s.logger.Warn("Failed to invalidate tenant cache",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
	}

	return tenant, nil
}

// DeleteTenant deletes a tenant configuration
func (s *TenantService) DeleteTenant(ctx context.Context, tenantID string) error {
	// Delete from metadata store
	if err := s.metadataStore.DeleteTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant from metadata store: %w", err)
	}

	s.logger.Info("Deleted tenant", zap.String("tenant_id", tenantID))

	// Invalidate cache
	cacheKey := s.tenantCacheKey(tenantID)
	if err := s.cache.Delete(ctx, cacheKey); err != nil {
		s.logger.Warn("Failed to invalidate tenant cache",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
	}

	return nil
}

// tenantCacheKey generates a cache key for tenant config
func (s *TenantService) tenantCacheKey(tenantID string) string {
	return fmt.Sprintf("tenant:config:%s", tenantID)
}
