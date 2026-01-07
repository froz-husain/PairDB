package service

import (
	"context"
	"sync"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/storage/memtable"
	"go.uber.org/zap"
)

// MemTableService manages in-memory data storage
type MemTableService struct {
	config      *MemTableConfig
	memTable    *MemTable
	immutableMT *MemTable
	logger      *zap.Logger
	mu          sync.RWMutex
	flushMu     sync.Mutex
}

// MemTableConfig holds memtable configuration
type MemTableConfig struct {
	MaxSize        int64
	FlushThreshold int64
	NumMemTables   int
}

// NewMemTableService creates a new memtable service
func NewMemTableService(cfg *MemTableConfig, logger *zap.Logger) *MemTableService {
	return &MemTableService{
		config:   cfg,
		memTable: NewMemTable(cfg.MaxSize),
		logger:   logger,
	}
}

// Put inserts or updates an entry in memtable
func (s *MemTableService) Put(ctx context.Context, entry *model.MemTableEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.memTable.Put(entry)
}

// Get retrieves an entry from memtable
func (s *MemTableService) Get(ctx context.Context, key string) (*model.MemTableEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check current memtable
	if entry, found := s.memTable.Get(key); found {
		return entry, true
	}

	// Check immutable memtable if exists
	if s.immutableMT != nil {
		if entry, found := s.immutableMT.Get(key); found {
			return entry, true
		}
	}

	return nil, false
}

// ShouldFlush checks if memtable should be flushed
func (s *MemTableService) ShouldFlush() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.memTable.Size() >= s.config.FlushThreshold
}

// Flush flushes memtable to SSTable
func (s *MemTableService) Flush(ctx context.Context, sstableSvc *SSTableService) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	// Make current memtable immutable
	s.mu.Lock()
	if s.memTable.Size() == 0 {
		s.mu.Unlock()
		return nil // Nothing to flush
	}

	s.immutableMT = s.memTable
	s.memTable = NewMemTable(s.config.MaxSize)
	immutable := s.immutableMT
	s.mu.Unlock()

	s.logger.Info("Starting memtable flush",
		zap.Int64("size", immutable.Size()),
		zap.Int("entries", immutable.Count()))

	// Write to SSTable
	if err := sstableSvc.WriteFromMemTable(ctx, immutable); err != nil {
		s.logger.Error("Failed to flush memtable", zap.Error(err))
		return err
	}

	// Clear immutable memtable
	s.mu.Lock()
	s.immutableMT = nil
	s.mu.Unlock()

	s.logger.Info("Memtable flush completed")
	return nil
}

// MemTable is an in-memory sorted table
type MemTable struct {
	data    *memtable.SkipList
	maxSize int64
	size    int64
	mu      sync.RWMutex
}

// NewMemTable creates a new memtable
func NewMemTable(maxSize int64) *MemTable {
	return &MemTable{
		data:    memtable.NewSkipList(),
		maxSize: maxSize,
	}
}

// Put inserts or updates an entry
func (mt *MemTable) Put(entry *model.MemTableEntry) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Estimate size
	entrySize := int64(len(entry.Key) + len(entry.Value) + 64) // Approximate

	mt.data.Insert(entry.Key, entry)
	mt.size += entrySize

	return nil
}

// Get retrieves an entry by key
func (mt *MemTable) Get(key string) (*model.MemTableEntry, bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	value, found := mt.data.Search(key)
	if !found {
		return nil, false
	}

	return value.(*model.MemTableEntry), true
}

// Size returns the current size of the memtable
func (mt *MemTable) Size() int64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.size
}

// Count returns the number of entries
func (mt *MemTable) Count() int {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.data.Len()
}

// Iterator returns an iterator over memtable entries
func (mt *MemTable) Iterator() *MemTableIterator {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return &MemTableIterator{
		skipList: mt.data,
		iter:     mt.data.Iterator(),
	}
}

// MemTableIterator iterates over memtable entries
type MemTableIterator struct {
	skipList *memtable.SkipList
	iter     *memtable.SkipListIterator
}

// Next moves to the next entry
func (it *MemTableIterator) Next() bool {
	return it.iter.Next()
}

// Entry returns the current entry
func (it *MemTableIterator) Entry() *model.MemTableEntry {
	value := it.iter.Value()
	if value == nil {
		return nil
	}
	return value.(*model.MemTableEntry)
}
