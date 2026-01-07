package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisIdempotencyStore implements IdempotencyStore for Redis
type RedisIdempotencyStore struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisIdempotencyStore creates a new Redis idempotency store
func NewRedisIdempotencyStore(host string, port int, password string, db int, logger *zap.Logger) (IdempotencyStore, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisIdempotencyStore{
		client: client,
		logger: logger,
	}, nil
}

// Get retrieves cached response
func (s *RedisIdempotencyStore) Get(ctx context.Context, key string) (interface{}, error) {
	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Deserialize JSON to interface{}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return result, nil
}

// Set stores response with TTL
func (s *RedisIdempotencyStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return s.client.Set(ctx, key, data, ttl).Err()
}

// Delete removes an idempotency key
func (s *RedisIdempotencyStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// Ping checks the Redis connection
func (s *RedisIdempotencyStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close closes the Redis client
func (s *RedisIdempotencyStore) Close() error {
	return s.client.Close()
}
