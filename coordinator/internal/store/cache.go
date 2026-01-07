package store

import (
	"sync"
	"time"

	"github.com/devrev/pairdb/coordinator/internal/model"
)

// Cache provides in-memory caching for tenant configurations
type Cache struct {
	tenantCache map[string]*cacheEntry
	mu          sync.RWMutex
	ttl         time.Duration
}

type cacheEntry struct {
	tenant    *model.Tenant
	expiresAt time.Time
}

// NewCache creates a new cache
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		tenantCache: make(map[string]*cacheEntry),
		ttl:         ttl,
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

// GetTenant retrieves a tenant from cache
func (c *Cache) GetTenant(tenantID string) (*model.Tenant, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.tenantCache[tenantID]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.tenant, true
}

// SetTenant stores a tenant in cache
func (c *Cache) SetTenant(tenantID string, tenant *model.Tenant) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tenantCache[tenantID] = &cacheEntry{
		tenant:    tenant,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// DeleteTenant removes a tenant from cache
func (c *Cache) DeleteTenant(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tenantCache, tenantID)
}

// Clear removes all entries from cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tenantCache = make(map[string]*cacheEntry)
}

// cleanup periodically removes expired entries
func (c *Cache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for tenantID, entry := range c.tenantCache {
			if now.After(entry.expiresAt) {
				delete(c.tenantCache, tenantID)
			}
		}
		c.mu.Unlock()
	}
}

// Size returns the number of entries in cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.tenantCache)
}
