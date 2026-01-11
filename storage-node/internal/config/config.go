package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	NodeID             string        `yaml:"node_id"`
	Host               string        `yaml:"host"`
	Port               int           `yaml:"port"`
	MaxConnections     int           `yaml:"max_connections"`
	ReadTimeout        time.Duration `yaml:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`
}

// CoordinatorConfig holds coordinator client configuration
type CoordinatorConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	VirtualNodes  int           `yaml:"virtual_nodes"`
	RetryInterval time.Duration `yaml:"retry_interval"`
	MaxRetries    int           `yaml:"max_retries"`
}

// Config represents the complete configuration for the storage node
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Coordinator CoordinatorConfig `yaml:"coordinator"`
	Storage     StorageConfig     `yaml:"storage"`
	CommitLog   CommitLogConfig   `yaml:"commit_log"`
	MemTable    MemTableConfig    `yaml:"mem_table"`
	SSTable     SSTableConfig     `yaml:"sstable"`
	Cache       CacheConfig       `yaml:"cache"`
	Compaction  CompactionConfig  `yaml:"compaction"`
	Gossip      GossipConfig      `yaml:"gossip"`
	Metrics     MetricsConfig     `yaml:"metrics"`
	Logging     LoggingConfig     `yaml:"logging"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	DataDir      string  `yaml:"data_dir"`
	CommitLogDir string  `yaml:"commit_log_dir"`
	SSTableDir   string  `yaml:"sstable_dir"`
	MaxDiskUsage float64 `yaml:"max_disk_usage"`
}

// CommitLogConfig holds commit log configuration
type CommitLogConfig struct {
	SegmentSize int64         `yaml:"segment_size"`
	MaxAge      time.Duration `yaml:"max_age"`
	SyncWrites  bool          `yaml:"sync_writes"`
	BufferSize  int           `yaml:"buffer_size"`
}

// MemTableConfig holds memtable configuration
type MemTableConfig struct {
	MaxSize        int64 `yaml:"max_size"`
	FlushThreshold int64 `yaml:"flush_threshold"`
	NumMemTables   int   `yaml:"num_mem_tables"`
}

// SSTableConfig holds SSTable configuration
type SSTableConfig struct {
	L0Size          int64   `yaml:"l0_size"`
	L1Size          int64   `yaml:"l1_size"`
	L2Size          int64   `yaml:"l2_size"`
	LevelMultiplier int     `yaml:"level_multiplier"`
	BloomFilterFP   float64 `yaml:"bloom_filter_fp"`
	BlockSize       int     `yaml:"block_size"`
	IndexInterval   int     `yaml:"index_interval"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	MaxSize         int64         `yaml:"max_size"`
	FrequencyWeight float64       `yaml:"frequency_weight"`
	RecencyWeight   float64       `yaml:"recency_weight"`
	AdaptiveWindow  time.Duration `yaml:"adaptive_window"`
}

// CompactionConfig holds compaction configuration
type CompactionConfig struct {
	L0Trigger int `yaml:"l0_trigger"`
	Workers   int `yaml:"workers"`
	Throttle  int `yaml:"throttle"`
}

// GossipConfig holds gossip protocol configuration
type GossipConfig struct {
	Enabled        bool          `yaml:"enabled"`
	BindPort       int           `yaml:"bind_port"`
	SeedNodes      []string      `yaml:"seed_nodes"`
	GossipInterval time.Duration `yaml:"gossip_interval"`
	ProbeTimeout   time.Duration `yaml:"probe_timeout"`
	ProbeInterval  time.Duration `yaml:"probe_interval"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadConfig loads configuration from a file
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults if not specified
	setDefaults(&cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for unspecified configuration
func setDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 50052
	}
	if cfg.Server.MaxConnections == 0 {
		cfg.Server.MaxConnections = 1000
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 10 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 10 * time.Second
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 30 * time.Second
	}

	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = "/var/lib/pairdb"
	}
	if cfg.Storage.CommitLogDir == "" {
		cfg.Storage.CommitLogDir = cfg.Storage.DataDir + "/commitlog"
	}
	if cfg.Storage.SSTableDir == "" {
		cfg.Storage.SSTableDir = cfg.Storage.DataDir + "/sstables"
	}
	if cfg.Storage.MaxDiskUsage == 0 {
		cfg.Storage.MaxDiskUsage = 0.9
	}

	if cfg.MemTable.MaxSize == 0 {
		cfg.MemTable.MaxSize = 67108864 // 64MB
	}
	if cfg.MemTable.FlushThreshold == 0 {
		cfg.MemTable.FlushThreshold = 60000000 // 60MB
	}

	if cfg.Cache.FrequencyWeight == 0 {
		cfg.Cache.FrequencyWeight = 0.5
	}
	if cfg.Cache.RecencyWeight == 0 {
		cfg.Cache.RecencyWeight = 0.5
	}

	// Coordinator defaults
	if cfg.Coordinator.Port == 0 {
		cfg.Coordinator.Port = 50051
	}
	if cfg.Coordinator.VirtualNodes == 0 {
		cfg.Coordinator.VirtualNodes = 150
	}
	if cfg.Coordinator.RetryInterval == 0 {
		cfg.Coordinator.RetryInterval = 5 * time.Second
	}
	if cfg.Coordinator.MaxRetries == 0 {
		cfg.Coordinator.MaxRetries = 10
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.NodeID == "" {
		return fmt.Errorf("server.node_id is required")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Storage.MaxDiskUsage < 0 || c.Storage.MaxDiskUsage > 1 {
		return fmt.Errorf("storage.max_disk_usage must be between 0 and 1")
	}
	return nil
}
