package service

import (
	"container/heap"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/storage/sstable"
	"go.uber.org/zap"
)

// CompactionService manages background SSTable compaction
type CompactionService struct {
	config           *CompactionConfig
	sstableService   *SSTableService
	logger           *zap.Logger
	jobQueue         chan *model.CompactionJob
	stopChan         chan struct{}
	wg               sync.WaitGroup
	bytesCompacted   uint64 // Atomic counter for metrics
	tablesCompacted  uint64 // Atomic counter for metrics
	compactionErrors uint64 // Atomic counter for metrics
}

// CompactionConfig holds compaction configuration
type CompactionConfig struct {
	L0Trigger int
	L0Size    int64
	L1Size    int64
	L2Size    int64
	Workers   int
	Throttle  int
	LevelMultiplier int
}

// NewCompactionService creates a new compaction service
func NewCompactionService(cfg *CompactionConfig, sstableSvc *SSTableService, logger *zap.Logger) *CompactionService {
	cs := &CompactionService{
		config:         cfg,
		sstableService: sstableSvc,
		logger:         logger,
		jobQueue:       make(chan *model.CompactionJob, 100),
		stopChan:       make(chan struct{}),
	}

	// Start compaction workers
	for i := 0; i < cfg.Workers; i++ {
		cs.wg.Add(1)
		go cs.compactionWorker(i)
	}

	// Start compaction scheduler
	go cs.compactionScheduler()

	return cs
}

// compactionScheduler periodically checks for compaction opportunities
func (s *CompactionService) compactionScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkCompactionNeeded()
		case <-s.stopChan:
			return
		}
	}
}

// checkCompactionNeeded checks if compaction is needed
func (s *CompactionService) checkCompactionNeeded() {
	// Check L0 compaction
	l0Tables := s.sstableService.GetTablesForLevel(model.L0)
	if len(l0Tables) >= s.config.L0Trigger {
		s.logger.Info("L0 compaction triggered",
			zap.Int("table_count", len(l0Tables)))

		job := &model.CompactionJob{
			JobID:       fmt.Sprintf("compact-l0-%d", time.Now().Unix()),
			Level:       model.L0,
			InputTables: l0Tables,
			OutputLevel: model.L1,
			StartedAt:   time.Now(),
			Status:      model.CompactionStatusPending,
		}

		select {
		case s.jobQueue <- job:
		default:
			s.logger.Warn("Compaction queue full")
		}
	}

	// Check other levels
	for level := model.L1; level < model.L4; level++ {
		if s.shouldCompactLevel(level) {
			s.triggerLevelCompaction(level)
		}
	}
}

// shouldCompactLevel checks if a level needs compaction
func (s *CompactionService) shouldCompactLevel(level model.SSTableLevel) bool {
	tables := s.sstableService.GetTablesForLevel(level)

	var totalSize int64
	for _, table := range tables {
		totalSize += table.Size
	}

	threshold := s.getLevelThreshold(level)
	return totalSize > threshold
}

// getLevelThreshold returns size threshold for a level
func (s *CompactionService) getLevelThreshold(level model.SSTableLevel) int64 {
	switch level {
	case model.L0:
		return s.config.L0Size
	case model.L1:
		return s.config.L1Size
	case model.L2:
		return s.config.L2Size
	default:
		baseSize := s.config.L2Size
		multiplier := int64(1)
		for i := model.L2; i < level; i++ {
			multiplier *= int64(s.config.LevelMultiplier)
		}
		return baseSize * multiplier
	}
}

// triggerLevelCompaction triggers compaction for a level
func (s *CompactionService) triggerLevelCompaction(level model.SSTableLevel) {
	tables := s.sstableService.GetTablesForLevel(level)
	if len(tables) == 0 {
		return
	}

	job := &model.CompactionJob{
		JobID:       fmt.Sprintf("compact-l%d-%d", level, time.Now().Unix()),
		Level:       level,
		InputTables: tables,
		OutputLevel: level + 1,
		StartedAt:   time.Now(),
		Status:      model.CompactionStatusPending,
	}

	select {
	case s.jobQueue <- job:
	default:
		s.logger.Warn("Compaction queue full")
	}
}

// compactionWorker processes compaction jobs
func (s *CompactionService) compactionWorker(workerID int) {
	defer s.wg.Done()

	s.logger.Info("Compaction worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case job := <-s.jobQueue:
			s.executeCompaction(job)
		case <-s.stopChan:
			return
		}
	}
}

// executeCompaction performs the actual compaction
func (s *CompactionService) executeCompaction(job *model.CompactionJob) {
	s.logger.Info("Starting compaction",
		zap.String("job_id", job.JobID),
		zap.Int("level", int(job.Level)),
		zap.Int("input_tables", len(job.InputTables)))

	job.Status = model.CompactionStatusRunning
	startTime := time.Now()

	// Perform actual compaction
	if err := s.mergeSSTables(job); err != nil {
		s.logger.Error("Compaction failed",
			zap.String("job_id", job.JobID),
			zap.Error(err))
		job.Status = model.CompactionStatusFailed
		return
	}

	job.Status = model.CompactionStatusCompleted
	duration := time.Since(startTime)

	s.logger.Info("Compaction completed",
		zap.String("job_id", job.JobID),
		zap.Duration("duration", duration))
}

// mergeSSTables merges multiple SSTables into one or more output SSTables
// Implements k-way merge with vector clock resolution and tombstone handling
func (s *CompactionService) mergeSSTables(job *model.CompactionJob) error {
	if len(job.InputTables) == 0 {
		return fmt.Errorf("no input tables to compact")
	}

	s.logger.Info("Merging SSTables",
		zap.Int("input_count", len(job.InputTables)),
		zap.Int("output_level", int(job.OutputLevel)))

	// Step 1: Open all input SSTable readers
	readers := make([]*sstable.SSTableReader, 0, len(job.InputTables))
	defer func() {
		for _, reader := range readers {
			reader.Close()
		}
	}()

	var totalInputSize int64
	for _, table := range job.InputTables {
		reader, err := sstable.NewSSTableReader(table.FilePath, table.IndexPath)
		if err != nil {
			return fmt.Errorf("failed to open input SSTable %s: %w", table.SSTableID, err)
		}
		readers = append(readers, reader)
		totalInputSize += table.Size
	}

	// Step 2: Create output SSTable writer
	outputPath := fmt.Sprintf("%s/sstable_l%d_%d.sst", s.sstableService.dataDir, job.OutputLevel, time.Now().UnixNano())
	writerConfig := &sstable.SSTableConfig{
		BloomFilterFP: 0.01,
		BlockSize:     4096,
		IndexInterval: 100,
	}
	writer, err := sstable.NewSSTableWriter(outputPath, writerConfig)
	if err != nil {
		return fmt.Errorf("failed to create output SSTable writer: %w", err)
	}
	defer writer.Close()

	// Step 3: Perform k-way merge using min heap
	entriesWritten := 0
	tombstonesRemoved := 0
	duplicatesResolved := 0
	bytesWritten := int64(0)

	merger := newKWayMerger(readers)
	var lastKey string
	var bestEntry *model.KeyValueEntry

	for merger.hasNext() {
		entry := merger.next()
		if entry == nil {
			continue
		}

		// Handle duplicate keys - keep the version with the latest vector clock
		if entry.Key == lastKey {
			// Compare vector clocks and keep the newer one
			if bestEntry != nil && s.compareVectorClocks(entry.VectorClock, bestEntry.VectorClock) > 0 {
				bestEntry = entry
			}
			duplicatesResolved++
			continue
		}

		// Write previous entry if exists
		if bestEntry != nil {
			if err := s.writeEntryIfNotTombstone(writer, bestEntry, &entriesWritten, &tombstonesRemoved, &bytesWritten); err != nil {
				return fmt.Errorf("failed to write entry: %w", err)
			}
		}

		// Start tracking new key
		lastKey = entry.Key
		bestEntry = entry
	}

	// Write final entry
	if bestEntry != nil {
		if err := s.writeEntryIfNotTombstone(writer, bestEntry, &entriesWritten, &tombstonesRemoved, &bytesWritten); err != nil {
			return fmt.Errorf("failed to write final entry: %w", err)
		}
	}

	// Step 4: Finalize output SSTable
	if err := writer.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize output SSTable: %w", err)
	}

	// Step 5: Create output SSTable metadata
	outputSSTable := &model.SSTableMetadata{
		SSTableID: fmt.Sprintf("sstable_l%d_%d", job.OutputLevel, time.Now().UnixNano()),
		Level:     int(job.OutputLevel),
		Size:      writer.Size(),
		CreatedAt: time.Now(),
		FilePath:  outputPath,
		IndexPath: outputPath + ".idx",
		BloomPath: outputPath + ".bloom",
	}

	// Step 6: Update SSTable metadata atomically
	s.sstableService.AddTable(job.OutputLevel, outputSSTable)

	// Step 7: Remove old SSTables
	inputIDs := make([]string, len(job.InputTables))
	for i, table := range job.InputTables {
		inputIDs[i] = table.SSTableID
	}
	s.sstableService.RemoveTables(job.Level, inputIDs)

	// Step 8: Delete old SSTable files
	for _, table := range job.InputTables {
		os.Remove(table.FilePath)
		os.Remove(table.IndexPath)
		os.Remove(table.BloomPath)
	}

	// Update metrics
	atomic.AddUint64(&s.bytesCompacted, uint64(totalInputSize))
	atomic.AddUint64(&s.tablesCompacted, uint64(len(job.InputTables)))

	s.logger.Info("Compaction merge completed",
		zap.Int("input_tables", len(job.InputTables)),
		zap.Int64("total_input_size", totalInputSize),
		zap.Int("entries_written", entriesWritten),
		zap.Int("tombstones_removed", tombstonesRemoved),
		zap.Int("duplicates_resolved", duplicatesResolved),
		zap.Int64("bytes_written", bytesWritten),
		zap.String("output_sstable", outputSSTable.SSTableID))

	return nil
}

// writeEntryIfNotTombstone writes an entry if it's not a tombstone
// Tombstones are filtered out during compaction
func (s *CompactionService) writeEntryIfNotTombstone(
	writer *sstable.SSTableWriter,
	entry *model.KeyValueEntry,
	entriesWritten *int,
	tombstonesRemoved *int,
	bytesWritten *int64,
) error {
	// Skip tombstones during compaction
	if entry.IsTombstone {
		*tombstonesRemoved++
		return nil
	}

	// Convert to MemTableEntry format for SSTable writer
	compositeKey := fmt.Sprintf("%s:%s", entry.TenantID, entry.Key)
	memEntry := &model.MemTableEntry{
		Key:         compositeKey,
		Value:       entry.Value,
		VectorClock: entry.VectorClock,
		Timestamp:   entry.Timestamp,
		IsTombstone: false,
	}

	if err := writer.Write(memEntry); err != nil {
		return err
	}

	*entriesWritten++
	*bytesWritten += int64(len(entry.Key) + len(entry.Value))
	return nil
}

// compareVectorClocks compares two vector clocks
// Returns: >0 if vc1 > vc2, <0 if vc1 < vc2, 0 if concurrent
func (s *CompactionService) compareVectorClocks(vc1, vc2 model.VectorClock) int {
	// Build maps for easier comparison
	map1 := make(map[string]int64)
	map2 := make(map[string]int64)

	for _, entry := range vc1.Entries {
		map1[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}
	for _, entry := range vc2.Entries {
		map2[entry.CoordinatorNodeID] = entry.LogicalTimestamp
	}

	vc1Greater := false
	vc2Greater := false

	// Check all nodes in vc1
	for nodeID, ts1 := range map1 {
		ts2, exists := map2[nodeID]
		if !exists {
			ts2 = 0
		}
		if ts1 > ts2 {
			vc1Greater = true
		} else if ts1 < ts2 {
			vc2Greater = true
		}
	}

	// Check nodes only in vc2
	for nodeID, ts2 := range map2 {
		if _, exists := map1[nodeID]; !exists {
			if ts2 > 0 {
				vc2Greater = true
			}
		}
	}

	// Determine relationship
	if vc1Greater && !vc2Greater {
		return 1 // vc1 > vc2
	} else if vc2Greater && !vc1Greater {
		return -1 // vc1 < vc2
	}
	return 0 // Concurrent
}

// kWayMerger implements k-way merge using a min heap
type kWayMerger struct {
	readers []*sstable.SSTableReader
	heap    *mergeHeap
	keys    [][]string // Keys for each reader
	indices []int      // Current index for each reader
}

// mergeEntry represents an entry in the merge heap
type mergeEntry struct {
	entry      *model.KeyValueEntry
	readerIdx  int
}

// mergeHeap implements heap.Interface for k-way merge
type mergeHeap []*mergeEntry

func (h mergeHeap) Len() int           { return len(h) }
func (h mergeHeap) Less(i, j int) bool { return h[i].entry.Key < h[j].entry.Key }
func (h mergeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x interface{}) {
	*h = append(*h, x.(*mergeEntry))
}

func (h *mergeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// newKWayMerger creates a new k-way merger
func newKWayMerger(readers []*sstable.SSTableReader) *kWayMerger {
	merger := &kWayMerger{
		readers: readers,
		heap:    &mergeHeap{},
		keys:    make([][]string, len(readers)),
		indices: make([]int, len(readers)),
	}

	// Get all keys from each reader
	for i, reader := range readers {
		merger.keys[i] = reader.GetAllKeys()
	}

	// Initialize heap with first entry from each reader
	heap.Init(merger.heap)
	for i := range readers {
		merger.advance(i)
	}

	return merger
}

// hasNext checks if there are more entries
func (m *kWayMerger) hasNext() bool {
	return m.heap.Len() > 0
}

// next returns the next entry in sorted order
func (m *kWayMerger) next() *model.KeyValueEntry {
	if !m.hasNext() {
		return nil
	}

	// Pop minimum entry
	minEntry := heap.Pop(m.heap).(*mergeEntry)
	readerIdx := minEntry.readerIdx

	// Advance the reader that provided this entry
	m.advance(readerIdx)

	return minEntry.entry
}

// advance advances a reader to its next entry
func (m *kWayMerger) advance(readerIdx int) {
	if m.indices[readerIdx] >= len(m.keys[readerIdx]) {
		return // No more entries from this reader
	}

	key := m.keys[readerIdx][m.indices[readerIdx]]
	m.indices[readerIdx]++

	// Read entry from SSTable
	entry, err := m.readers[readerIdx].Get(key)
	if err != nil || entry == nil {
		return // Skip on error
	}

	// Push to heap
	heap.Push(m.heap, &mergeEntry{
		entry:     entry,
		readerIdx: readerIdx,
	})
}

// Stop stops the compaction service
func (s *CompactionService) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}
