package store

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// InMemoryCache implements Cache using an in-memory map
type InMemoryCache struct {
	data   map[string]*cacheItem
	mu     sync.RWMutex
	maxSize int
	logger *zap.Logger
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache(maxSize int, logger *zap.Logger) Cache {
	cache := &InMemoryCache{
		data:    make(map[string]*cacheItem),
		maxSize: maxSize,
		logger:  logger,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from cache
func (c *InMemoryCache) Get(ctx context.Context, key string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[key]
	if !exists {
		return nil, ErrNotFound
	}

	// Check if expired
	if time.Now().After(item.expiresAt) {
		return nil, ErrNotFound
	}

	return item.value, nil
}

// Set stores a value in cache with TTL
func (c *InMemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict (simple size-based eviction)
	if len(c.data) >= c.maxSize {
		// Remove oldest expired item or just the first one
		for k, v := range c.data {
			if time.Now().After(v.expiresAt) {
				delete(c.data, k)
				break
			}
		}
		// If still at max size, remove any item
		if len(c.data) >= c.maxSize {
			for k := range c.data {
				delete(c.data, k)
				break
			}
		}
	}

	c.data[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes a value from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	return nil
}

// cleanup periodically removes expired entries
func (c *InMemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.data {
			if now.After(item.expiresAt) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

// Size returns the number of items in cache
func (c *InMemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}
