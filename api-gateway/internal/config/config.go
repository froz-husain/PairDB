// Package config provides configuration management for the API Gateway.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the API Gateway.
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Coordinator CoordinatorConfig `mapstructure:"coordinator"`
	RateLimiter RateLimiterConfig `mapstructure:"rate_limiter"`
	Metrics     MetricsConfig     `mapstructure:"metrics"`
	Logging     LoggingConfig     `mapstructure:"logging"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// CoordinatorConfig holds gRPC client configuration for the Coordinator service.
type CoordinatorConfig struct {
	Endpoints             []string      `mapstructure:"endpoints"`
	Timeout               time.Duration `mapstructure:"timeout"`
	MaxRetries            int           `mapstructure:"max_retries"`
	RetryBackoff          time.Duration `mapstructure:"retry_backoff"`
	KeepaliveTime         time.Duration `mapstructure:"keepalive_time"`
	KeepaliveTimeout      time.Duration `mapstructure:"keepalive_timeout"`
	MaxReceiveMessageSize int           `mapstructure:"max_receive_message_size"`
	MaxSendMessageSize    int           `mapstructure:"max_send_message_size"`
}

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	BurstSize         int     `mapstructure:"burst_size"`
}

// MetricsConfig holds Prometheus metrics configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// Load reads configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/api-gateway/")
	}

	// Read environment variables
	v.SetEnvPrefix("API_GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore if not found, use defaults/env)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "30s")

	// Coordinator defaults
	v.SetDefault("coordinator.endpoints", []string{"localhost:50051"})
	v.SetDefault("coordinator.timeout", "30s")
	v.SetDefault("coordinator.max_retries", 3)
	v.SetDefault("coordinator.retry_backoff", "100ms")
	v.SetDefault("coordinator.keepalive_time", "30s")
	v.SetDefault("coordinator.keepalive_timeout", "10s")
	v.SetDefault("coordinator.max_receive_message_size", 16777216)
	v.SetDefault("coordinator.max_send_message_size", 16777216)

	// Rate limiter defaults
	v.SetDefault("rate_limiter.enabled", true)
	v.SetDefault("rate_limiter.requests_per_second", 1000.0)
	v.SetDefault("rate_limiter.burst_size", 100)

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9090)
	v.SetDefault("metrics.path", "/metrics")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if len(c.Coordinator.Endpoints) == 0 {
		return fmt.Errorf("at least one coordinator endpoint is required")
	}

	if c.Coordinator.Timeout <= 0 {
		return fmt.Errorf("coordinator timeout must be positive")
	}

	if c.RateLimiter.Enabled {
		if c.RateLimiter.RequestsPerSecond <= 0 {
			return fmt.Errorf("rate limiter requests per second must be positive")
		}
		if c.RateLimiter.BurstSize <= 0 {
			return fmt.Errorf("rate limiter burst size must be positive")
		}
	}

	if c.Metrics.Enabled {
		if c.Metrics.Port <= 0 || c.Metrics.Port > 65535 {
			return fmt.Errorf("invalid metrics port: %d", c.Metrics.Port)
		}
	}

	return nil
}

