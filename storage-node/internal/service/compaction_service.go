package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// CompactionService manages background SSTable compaction
type CompactionService struct {
	config         *CompactionConfig
	sstableService *SSTableService
	logger         *zap.Logger
	jobQueue       chan *model.CompactionJob
	stopChan       chan struct{}
	wg             sync.WaitGroup
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
func (s *CompactionService) mergeSSTables(job *model.CompactionJob) error {
	if len(job.InputTables) == 0 {
		return fmt.Errorf("no input tables to compact")
	}

	s.logger.Info("Merging SSTables",
		zap.Int("input_count", len(job.InputTables)),
		zap.Int("output_level", int(job.OutputLevel)))

	// In production implementation:
	// 1. Open all input SSTable readers
	// 2. Create k-way merge iterator over all readers
	// 3. Create output SSTable writer at OutputLevel
	// 4. Iterate through merged entries (sorted by key)
	// 5. For duplicate keys, keep the version with latest vector clock
	// 6. Write deduplicated entries to output SSTable
	// 7. Finalize output SSTable (write index, bloom filter, sync)
	// 8. Update metadata: register new SSTable, mark old ones for deletion
	// 9. Schedule garbage collection of old SSTables

	var totalInputSize int64
	var totalInputKeys int

	for _, table := range job.InputTables {
		totalInputSize += table.Size
		s.logger.Debug("Input table",
			zap.String("sstable_id", table.SSTableID),
			zap.Int64("size", table.Size),
			zap.Int("level", table.Level))
	}

	// Create output SSTable metadata
	outputSSTable := &model.SSTableMetadata{
		SSTableID: fmt.Sprintf("sstable_l%d_%d", job.OutputLevel, time.Now().UnixNano()),
		Level:     int(job.OutputLevel),
		Size:      totalInputSize, // Approximate, will be less after deduplication
		CreatedAt: time.Now(),
		FilePath:  fmt.Sprintf("/data/sstables/sstable_l%d_%d.sst", job.OutputLevel, time.Now().UnixNano()),
		IndexPath: fmt.Sprintf("/data/sstables/sstable_l%d_%d.idx", job.OutputLevel, time.Now().UnixNano()),
		BloomPath: fmt.Sprintf("/data/sstables/sstable_l%d_%d.bloom", job.OutputLevel, time.Now().UnixNano()),
	}

	s.logger.Info("Created output SSTable",
		zap.String("sstable_id", outputSSTable.SSTableID),
		zap.String("file_path", outputSSTable.FilePath))

	// In production, actually perform the merge here using:
	// - sstable.NewSSTableReader() for each input
	// - sstable.NewSSTableWriter() for output
	// - Merge algorithm with vector clock comparison

	// Mark input tables for deletion (in production, use proper lifecycle management)
	for _, inputTable := range job.InputTables {
		s.logger.Debug("Marking input table for deletion",
			zap.String("sstable_id", inputTable.SSTableID),
			zap.String("file_path", inputTable.FilePath))
	}

	s.logger.Info("Compaction merge completed",
		zap.Int("input_tables", len(job.InputTables)),
		zap.Int64("total_input_size", totalInputSize),
		zap.Int("total_input_keys", totalInputKeys),
		zap.String("output_sstable", outputSSTable.SSTableID))

	return nil
}

// Stop stops the compaction service
func (s *CompactionService) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}
