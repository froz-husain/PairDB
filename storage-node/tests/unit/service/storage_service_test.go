package service_test

import (
	"context"
	"testing"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupStorageService creates a test storage service
func setupStorageService(t *testing.T) *service.StorageService {
	logger := zap.NewNop()

	// Create temporary directory for testing
	tmpDir := t.TempDir()

	// Create commit log service
	commitLogCfg := &service.CommitLogConfig{
		DataDir:        tmpDir + "/commitlog",
		SegmentSize:    1024 * 1024, // 1MB for testing
		SyncInterval:   100,
		RetentionHours: 24,
	}
	commitLogSvc := service.NewCommitLogService(commitLogCfg, logger)

	// Create memtable service
	memTableCfg := &service.MemTableConfig{
		MaxSize:        1024 * 1024, // 1MB
		FlushThreshold: 512 * 1024,  // 512KB
		NumMemTables:   2,
	}
	memTableSvc := service.NewMemTableService(memTableCfg, logger)

	// Create SSTable service
	sstableCfg := &service.SSTableConfig{
		DataDir:      tmpDir + "/sstables",
		L0Trigger:    4,
		MaxFileSize:  1024 * 1024,
		BloomFPR:     0.01,
		CacheSize:    1024 * 1024,
	}
	sstableSvc := service.NewSSTableService(sstableCfg, logger)

	// Create cache service
	cacheCfg := &service.CacheConfig{
		MaxSize:     1024 * 1024,
		EvictionAlg: "lru",
		TTL:         3600,
	}
	cacheSvc := service.NewCacheService(cacheCfg, logger)

	// Create vector clock service
	vectorClockCfg := &service.VectorClockConfig{
		NodeID: "test-node",
	}
	vectorClockSvc := service.NewVectorClockService(vectorClockCfg, logger)

	return service.NewStorageService(
		commitLogSvc,
		memTableSvc,
		sstableSvc,
		cacheSvc,
		vectorClockSvc,
		logger,
		"test-node",
	)
}

func TestStorageService_Write(t *testing.T) {
	svc := setupStorageService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		tenantID    string
		key         string
		value       []byte
		vectorClock model.VectorClock
		wantErr     bool
	}{
		{
			name:     "write valid entry",
			tenantID: "tenant1",
			key:      "key1",
			value:    []byte("value1"),
			vectorClock: model.VectorClock{
				Entries: []model.VectorClockEntry{
					{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
				},
			},
			wantErr: false,
		},
		{
			name:        "write empty tenant ID",
			tenantID:    "",
			key:         "key1",
			value:       []byte("value1"),
			vectorClock: model.VectorClock{},
			wantErr:     true,
		},
		{
			name:        "write empty key",
			tenantID:    "tenant1",
			key:         "",
			value:       []byte("value1"),
			vectorClock: model.VectorClock{},
			wantErr:     true,
		},
		{
			name:        "write nil value",
			tenantID:    "tenant1",
			key:         "key1",
			value:       nil,
			vectorClock: model.VectorClock{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.Write(ctx, tt.tenantID, tt.key, tt.value, tt.vectorClock)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, resp.Success)
		})
	}
}

func TestStorageService_Read(t *testing.T) {
	svc := setupStorageService(t)
	ctx := context.Background()

	// Write test data
	tenantID := "tenant1"
	key := "key1"
	value := []byte("value1")
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
		},
	}

	_, err := svc.Write(ctx, tenantID, key, value, vectorClock)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tenantID  string
		key       string
		wantValue []byte
		wantErr   bool
	}{
		{
			name:      "read existing key",
			tenantID:  tenantID,
			key:       key,
			wantValue: value,
			wantErr:   false,
		},
		{
			name:      "read non-existing key",
			tenantID:  tenantID,
			key:       "nonexistent",
			wantValue: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.Read(ctx, tt.tenantID, tt.key)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, resp.Success)
			assert.Equal(t, tt.wantValue, resp.Value)
		})
	}
}

func TestStorageService_WriteRead(t *testing.T) {
	svc := setupStorageService(t)
	ctx := context.Background()

	// Write multiple entries
	entries := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	tenantID := "tenant1"
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
		},
	}

	// Write all entries
	for key, value := range entries {
		_, err := svc.Write(ctx, tenantID, key, value, vectorClock)
		require.NoError(t, err)
	}

	// Read all entries
	for key, expectedValue := range entries {
		resp, err := svc.Read(ctx, tenantID, key)
		require.NoError(t, err)
		assert.Equal(t, expectedValue, resp.Value)
	}
}

func TestStorageService_Repair(t *testing.T) {
	svc := setupStorageService(t)
	ctx := context.Background()

	tenantID := "tenant1"
	key := "key1"
	value := []byte("repaired_value")
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 2},
		},
	}

	err := svc.Repair(ctx, tenantID, key, value, vectorClock)
	require.NoError(t, err)

	// Verify repaired value can be read
	resp, err := svc.Read(ctx, tenantID, key)
	require.NoError(t, err)
	assert.Equal(t, value, resp.Value)
}

func TestStorageService_CacheEviction(t *testing.T) {
	svc := setupStorageService(t)
	ctx := context.Background()

	tenantID := "tenant1"
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
		},
	}

	// First read - cache miss
	_, err := svc.Write(ctx, tenantID, "key1", []byte("value1"), vectorClock)
	require.NoError(t, err)

	resp, err := svc.Read(ctx, tenantID, "key1")
	require.NoError(t, err)
	assert.Equal(t, "memtable", resp.Source)

	// Second read - cache hit
	resp, err = svc.Read(ctx, tenantID, "key1")
	require.NoError(t, err)
	assert.Equal(t, "cache", resp.Source)
}

func BenchmarkStorageService_Write(b *testing.B) {
	logger := zap.NewNop()
	tmpDir := b.TempDir()

	commitLogCfg := &service.CommitLogConfig{
		DataDir:        tmpDir + "/commitlog",
		SegmentSize:    1024 * 1024,
		SyncInterval:   100,
		RetentionHours: 24,
	}
	commitLogSvc := service.NewCommitLogService(commitLogCfg, logger)

	memTableCfg := &service.MemTableConfig{
		MaxSize:        10 * 1024 * 1024,
		FlushThreshold: 8 * 1024 * 1024,
		NumMemTables:   2,
	}
	memTableSvc := service.NewMemTableService(memTableCfg, logger)

	sstableCfg := &service.SSTableConfig{
		DataDir:      tmpDir + "/sstables",
		L0Trigger:    4,
		MaxFileSize:  1024 * 1024,
		BloomFPR:     0.01,
		CacheSize:    1024 * 1024,
	}
	sstableSvc := service.NewSSTableService(sstableCfg, logger)

	cacheCfg := &service.CacheConfig{
		MaxSize:     1024 * 1024,
		EvictionAlg: "lru",
		TTL:         3600,
	}
	cacheSvc := service.NewCacheService(cacheCfg, logger)

	vectorClockCfg := &service.VectorClockConfig{
		NodeID: "test-node",
	}
	vectorClockSvc := service.NewVectorClockService(vectorClockCfg, logger)

	svc := service.NewStorageService(
		commitLogSvc,
		memTableSvc,
		sstableSvc,
		cacheSvc,
		vectorClockSvc,
		logger,
		"test-node",
	)

	ctx := context.Background()
	tenantID := "tenant1"
	value := []byte("value")
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i))
		svc.Write(ctx, tenantID, key, value, vectorClock)
	}
}

func BenchmarkStorageService_Read(b *testing.B) {
	logger := zap.NewNop()
	tmpDir := b.TempDir()

	commitLogCfg := &service.CommitLogConfig{
		DataDir:        tmpDir + "/commitlog",
		SegmentSize:    1024 * 1024,
		SyncInterval:   100,
		RetentionHours: 24,
	}
	commitLogSvc := service.NewCommitLogService(commitLogCfg, logger)

	memTableCfg := &service.MemTableConfig{
		MaxSize:        10 * 1024 * 1024,
		FlushThreshold: 8 * 1024 * 1024,
		NumMemTables:   2,
	}
	memTableSvc := service.NewMemTableService(memTableCfg, logger)

	sstableCfg := &service.SSTableConfig{
		DataDir:      tmpDir + "/sstables",
		L0Trigger:    4,
		MaxFileSize:  1024 * 1024,
		BloomFPR:     0.01,
		CacheSize:    1024 * 1024,
	}
	sstableSvc := service.NewSSTableService(sstableCfg, logger)

	cacheCfg := &service.CacheConfig{
		MaxSize:     1024 * 1024,
		EvictionAlg: "lru",
		TTL:         3600,
	}
	cacheSvc := service.NewCacheService(cacheCfg, logger)

	vectorClockCfg := &service.VectorClockConfig{
		NodeID: "test-node",
	}
	vectorClockSvc := service.NewVectorClockService(vectorClockCfg, logger)

	svc := service.NewStorageService(
		commitLogSvc,
		memTableSvc,
		sstableSvc,
		cacheSvc,
		vectorClockSvc,
		logger,
		"test-node",
	)

	ctx := context.Background()
	tenantID := "tenant1"
	value := []byte("value")
	vectorClock := model.VectorClock{
		Entries: []model.VectorClockEntry{
			{CoordinatorNodeID: "coord1", LogicalTimestamp: 1},
		},
	}

	// Pre-populate with data
	for i := 0; i < 1000; i++ {
		key := "key" + string(rune(i))
		svc.Write(ctx, tenantID, key, value, vectorClock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i%1000))
		svc.Read(ctx, tenantID, key)
	}
}
