package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/storage/sstable"
	"go.uber.org/zap"
)

// SSTableService manages persistent storage in SSTables
type SSTableService struct {
	config  *SSTableConfig
	dataDir string
	logger  *zap.Logger
	levels  map[model.SSTableLevel][]*model.SSTableMetadata
	mu      sync.RWMutex
}

// SSTableConfig holds SSTable configuration
type SSTableConfig struct {
	L0Size          int64
	L1Size          int64
	L2Size          int64
	LevelMultiplier int
	BloomFilterFP   float64
	BlockSize       int
	IndexInterval   int
}

// NewSSTableService creates a new SSTable service
func NewSSTableService(cfg *SSTableConfig, dataDir string, logger *zap.Logger) *SSTableService {
	// Ensure level directories exist
	for level := model.L0; level <= model.L4; level++ {
		levelDir := filepath.Join(dataDir, fmt.Sprintf("l%d", level))
		os.MkdirAll(levelDir, 0755)
	}

	return &SSTableService{
		config:  cfg,
		dataDir: dataDir,
		logger:  logger,
		levels:  make(map[model.SSTableLevel][]*model.SSTableMetadata),
	}
}

// WriteFromMemTable writes memtable contents to new L0 SSTable
func (s *SSTableService) WriteFromMemTable(ctx context.Context, memTable *MemTable) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate SSTable ID
	sstableID := fmt.Sprintf("sstable-%d", time.Now().UnixNano())
	levelDir := filepath.Join(s.dataDir, "l0")
	filePath := filepath.Join(levelDir, sstableID+".sst")

	// Create SSTable writer
	writerCfg := &sstable.SSTableConfig{
		BloomFilterFP: s.config.BloomFilterFP,
		BlockSize:     s.config.BlockSize,
		IndexInterval: s.config.IndexInterval,
	}

	writer, err := sstable.NewSSTableWriter(filePath, writerCfg)
	if err != nil {
		return fmt.Errorf("failed to create sstable writer: %w", err)
	}
	defer writer.Close()

	// Write entries from memtable
	iterator := memTable.Iterator()
	var keyRange model.KeyRange
	entryCount := 0

	for iterator.Next() {
		entry := iterator.Entry()

		if entryCount == 0 {
			keyRange.StartKey = entry.Key
		}
		keyRange.EndKey = entry.Key

		if err := writer.Write(entry); err != nil {
			return fmt.Errorf("failed to write entry: %w", err)
		}
		entryCount++
	}

	// Finalize SSTable
	if err := writer.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize sstable: %w", err)
	}

	// Create metadata
	metadata := &model.SSTableMetadata{
		SSTableID: sstableID,
		Level:     int(model.L0),
		Size:      writer.Size(),
		KeyRange:  keyRange,
		CreatedAt: time.Now(),
		FilePath:  filePath,
		IndexPath: filePath + ".idx",
		BloomPath: filePath + ".bloom",
	}

	// Add to level 0
	s.levels[model.L0] = append(s.levels[model.L0], metadata)

	s.logger.Info("Created new SSTable",
		zap.String("sstable_id", sstableID),
		zap.Int("level", 0),
		zap.Int("entries", entryCount),
		zap.Int64("size", metadata.Size))

	return nil
}

// Get retrieves a value from SSTables
func (s *SSTableService) Get(ctx context.Context, tenantID string, key string) (*model.KeyValueEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	compositeKey := fmt.Sprintf("%s:%s", tenantID, key)
	var latestEntry *model.KeyValueEntry
	var latestTimestamp int64

	// Search from L0 to higher levels
	for level := model.L0; level <= model.L4; level++ {
		tables := s.levels[level]

		for _, table := range tables {
			// Check if key is in range
			if !s.keyInRange(compositeKey, table.KeyRange) {
				continue
			}

			// Check bloom filter
			bloomFilter, err := sstable.LoadBloomFilter(table.BloomPath)
			if err != nil {
				s.logger.Warn("Failed to load bloom filter", zap.Error(err))
				continue
			}

			if !bloomFilter.MayContain(compositeKey) {
				continue // Definitely not in this SSTable
			}

			// Search in SSTable
			reader, err := sstable.NewSSTableReader(table.FilePath, table.IndexPath)
			if err != nil {
				s.logger.Error("Failed to open sstable", zap.Error(err))
				continue
			}

			entry, err := reader.Get(compositeKey)
			reader.Close()

			if err != nil {
				continue
			}

			if entry != nil && entry.Timestamp > latestTimestamp {
				latestEntry = entry
				latestTimestamp = entry.Timestamp
			}
		}
	}

	return latestEntry, nil
}

// ScanKeysInRange scans keys in SSTables whose hash falls within the specified range
// Used for streaming during node addition/removal
func (s *SSTableService) ScanKeysInRange(ctx context.Context, startHash, endHash uint64, hashFunc func(string) uint64) ([]*model.KeyValueEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*model.KeyValueEntry
	seenKeys := make(map[string]int64) // key -> timestamp (for deduplication)

	// Scan all levels from L0 to L4
	for level := model.L0; level <= model.L4; level++ {
		tables := s.levels[level]

		for _, table := range tables {
			// Open SSTable reader
			reader, err := sstable.NewSSTableReader(table.FilePath, table.IndexPath)
			if err != nil {
				s.logger.Error("Failed to open sstable for scanning",
					zap.String("sstable_id", table.SSTableID),
					zap.Error(err))
				continue
			}

			// Get all keys from this SSTable and check each one
			keys := reader.GetAllKeys()
			for _, key := range keys {
				// Compute hash of the key
				keyHash := hashFunc(key)

				// Check if hash falls in range (handle wrap-around)
				inRange := false
				if endHash > startHash {
					// Normal range: [startHash, endHash)
					inRange = keyHash >= startHash && keyHash < endHash
				} else {
					// Wrap-around range: [startHash, MAX] or [0, endHash)
					inRange = keyHash >= startHash || keyHash < endHash
				}

				if inRange {
					// Get the entry
					entry, err := reader.Get(key)
					if err != nil || entry == nil {
						continue
					}

					// Check if we've seen this key before
					compositeKey := fmt.Sprintf("%s:%s", entry.TenantID, entry.Key)
					if existingTS, seen := seenKeys[compositeKey]; seen {
						// Keep the newer version
						if entry.Timestamp > existingTS {
							// Remove old version and add new one
							for i, e := range results {
								resultKey := fmt.Sprintf("%s:%s", e.TenantID, e.Key)
								if resultKey == compositeKey {
									results[i] = entry
									seenKeys[compositeKey] = entry.Timestamp
									break
								}
							}
						}
					} else {
						// New key
						results = append(results, entry)
						seenKeys[compositeKey] = entry.Timestamp
					}
				}
			}

			reader.Close()
		}
	}

	s.logger.Debug("Scanned SSTables for keys in range",
		zap.Uint64("start_hash", startHash),
		zap.Uint64("end_hash", endHash),
		zap.Int("keys_found", len(results)))

	return results, nil
}

// keyInRange checks if key falls within range
func (s *SSTableService) keyInRange(key string, keyRange model.KeyRange) bool {
	return key >= keyRange.StartKey && key <= keyRange.EndKey
}

// GetTablesForLevel returns SSTables for a specific level
func (s *SSTableService) GetTablesForLevel(level model.SSTableLevel) []*model.SSTableMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.levels[level]
}

// AddTable adds an SSTable to a level
func (s *SSTableService) AddTable(level model.SSTableLevel, table *model.SSTableMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.levels[level] = append(s.levels[level], table)
}

// RemoveTables removes SSTables from a level
func (s *SSTableService) RemoveTables(level model.SSTableLevel, tableIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create set of IDs to remove
	removeSet := make(map[string]bool)
	for _, id := range tableIDs {
		removeSet[id] = true
	}

	// Filter tables
	filtered := make([]*model.SSTableMetadata, 0)
	for _, table := range s.levels[level] {
		if !removeSet[table.SSTableID] {
			filtered = append(filtered, table)
		}
	}

	s.levels[level] = filtered
}
