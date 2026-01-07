package config

import (
	"errors"
	"time"
)

// Config represents the coordinator service configuration
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	HashRing    HashRingConfig    `mapstructure:"hash_ring"`
	Consistency ConsistencyConfig `mapstructure:"consistency"`
	Cache       CacheConfig       `mapstructure:"cache"`
	Metrics     MetricsConfig     `mapstructure:"metrics"`
	Logging     LoggingConfig     `mapstructure:"logging"`
}

// ServerConfig represents gRPC server configuration
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	NodeID          string        `mapstructure:"node_id"`
	MaxConnections  int           `mapstructure:"max_connections"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig represents PostgreSQL metadata store configuration
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	MaxConnections  int           `mapstructure:"max_connections"`
	MinConnections  int           `mapstructure:"min_connections"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// RedisConfig represents Redis idempotency store configuration
type RedisConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	MaxRetries   int    `mapstructure:"max_retries"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// HashRingConfig represents consistent hashing configuration
type HashRingConfig struct {
	VirtualNodes   int           `mapstructure:"virtual_nodes"`
	UpdateInterval time.Duration `mapstructure:"update_interval"`
}

// ConsistencyConfig represents consistency level configuration
type ConsistencyConfig struct {
	DefaultLevel string        `mapstructure:"default_level"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	RepairAsync  bool          `mapstructure:"repair_async"`
}

// CacheConfig represents cache configuration
type CacheConfig struct {
	TenantConfigTTL time.Duration `mapstructure:"tenant_config_ttl"`
	MaxSize         int           `mapstructure:"max_size"`
}

// MetricsConfig represents Prometheus metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Host == "" {
		return errors.New("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if c.Server.NodeID == "" {
		return errors.New("server.node_id is required")
	}
	if c.Database.Host == "" {
		return errors.New("database.host is required")
	}
	if c.Database.Database == "" {
		return errors.New("database.database is required")
	}
	if c.Database.User == "" {
		return errors.New("database.user is required")
	}
	if c.Redis.Host == "" {
		return errors.New("redis.host is required")
	}
	if c.HashRing.VirtualNodes <= 0 {
		return errors.New("hash_ring.virtual_nodes must be positive")
	}
	if c.Consistency.DefaultLevel == "" {
		c.Consistency.DefaultLevel = "quorum"
	}
	if !isValidConsistencyLevel(c.Consistency.DefaultLevel) {
		return errors.New("consistency.default_level must be one of: one, quorum, all")
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	return nil
}

// isValidConsistencyLevel checks if the consistency level is valid
func isValidConsistencyLevel(level string) bool {
	switch level {
	case "one", "quorum", "all":
		return true
	default:
		return false
	}
}

// DefaultConfig returns default configuration values
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            50051,
			NodeID:          "coordinator-1",
			MaxConnections:  1000,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "pairdb_metadata",
			User:            "coordinator",
			Password:        "",
			MaxConnections:  50,
			MinConnections:  10,
			ConnMaxLifetime: 30 * time.Minute,
		},
		Redis: RedisConfig{
			Host:         "localhost",
			Port:         6379,
			Password:     "",
			DB:           0,
			MaxRetries:   3,
			PoolSize:     100,
			MinIdleConns: 10,
		},
		HashRing: HashRingConfig{
			VirtualNodes:   150,
			UpdateInterval: 30 * time.Second,
		},
		Consistency: ConsistencyConfig{
			DefaultLevel: "quorum",
			WriteTimeout: 5 * time.Second,
			ReadTimeout:  3 * time.Second,
			RepairAsync:  true,
		},
		Cache: CacheConfig{
			TenantConfigTTL: 5 * time.Minute,
			MaxSize:         10000,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
