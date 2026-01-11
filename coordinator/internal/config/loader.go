package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/viper"
)

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Set defaults
	cfg := DefaultConfig()

	// Set up viper
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Read config file (optional - if file doesn't exist, continue with defaults)
	if err := viper.ReadInConfig(); err != nil {
		// Config file is optional if environment variables are set
		fmt.Printf("Warning: Could not read config file %s: %v. Using defaults and environment variables.\n", configPath, err)
	} else {
		// Unmarshal file contents
		if err := viper.Unmarshal(cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	// Override with environment variables (these take precedence)
	applyEnvironmentOverrides(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// applyEnvironmentOverrides applies environment variable overrides to config
func applyEnvironmentOverrides(cfg *Config) {
	// Server configuration
	if nodeID := os.Getenv("COORDINATOR_NODE_ID"); nodeID != "" {
		cfg.Server.NodeID = nodeID
	}
	if host := os.Getenv("SERVER_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}

	// Database configuration
	if dbHost := os.Getenv("DATABASE_HOST"); dbHost != "" {
		cfg.Database.Host = dbHost
	}
	if dbPort := os.Getenv("DATABASE_PORT"); dbPort != "" {
		if p, err := strconv.Atoi(dbPort); err == nil {
			cfg.Database.Port = p
		}
	}
	if dbName := os.Getenv("DATABASE_NAME"); dbName != "" {
		cfg.Database.Database = dbName
	}
	if dbUser := os.Getenv("DATABASE_USER"); dbUser != "" {
		cfg.Database.User = dbUser
	}
	if dbPassword := os.Getenv("DATABASE_PASSWORD"); dbPassword != "" {
		cfg.Database.Password = dbPassword
	}

	// Redis configuration
	if redisHost := os.Getenv("REDIS_HOST"); redisHost != "" {
		cfg.Redis.Host = redisHost
	}
	if redisPort := os.Getenv("REDIS_PORT"); redisPort != "" {
		if p, err := strconv.Atoi(redisPort); err == nil {
			cfg.Redis.Port = p
		}
	}
	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		cfg.Redis.Password = redisPassword
	}

	// Logging configuration
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		cfg.Logging.Level = logLevel
	}
}
