package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/util"
	"go.uber.org/zap"
)

// CommitLogService manages write-ahead logging for durability
type CommitLogService struct {
	config         *CommitLogConfig
	currentFile    *os.File
	logger         *zap.Logger
	mu             sync.Mutex
	dataDir        string
	segmentID      int64
	stopChan       chan struct{}
	sequenceNumber uint64 // Atomic counter for sequence numbers
}

// CommitLogConfig holds commit log configuration
type CommitLogConfig struct {
	SegmentSize int64
	MaxAge      time.Duration
	SyncWrites  bool
	BufferSize  int
}

// NewCommitLogService creates a new commit log service
func NewCommitLogService(cfg *CommitLogConfig, dataDir string, logger *zap.Logger) (*CommitLogService, error) {
	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create commit log directory: %w", err)
	}

	cls := &CommitLogService{
		config:    cfg,
		logger:    logger,
		dataDir:   dataDir,
		segmentID: time.Now().Unix(),
		stopChan:  make(chan struct{}),
	}

	if err := cls.openNewSegment(); err != nil {
		return nil, fmt.Errorf("failed to open commit log segment: %w", err)
	}

	// Start rotation checker
	go cls.rotationChecker()

	return cls, nil
}

// Append appends an entry to the commit log with checksum and sequence number
func (s *CommitLogService) Append(ctx context.Context, entry *model.CommitLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Assign sequence number (monotonically increasing)
	entry.SequenceNumber = atomic.AddUint64(&s.sequenceNumber, 1)

	// Serialize entry (without checksum first)
	entry.Checksum = 0
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Compute checksum of serialized data
	entry.Checksum = util.ComputeChecksum(data)

	// Re-serialize with checksum
	data, err = json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry with checksum: %w", err)
	}

	// Append newline for easier parsing
	data = append(data, '\n')

	// Write to file
	if _, err := s.currentFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to commit log: %w", err)
	}

	// Sync if configured
	if s.config.SyncWrites {
		if err := s.currentFile.Sync(); err != nil {
			return fmt.Errorf("failed to sync commit log: %w", err)
		}
	}

	return nil
}

// openNewSegment creates a new commit log segment
func (s *CommitLogService) openNewSegment() error {
	// Close current file if exists
	if s.currentFile != nil {
		s.currentFile.Close()
	}

	// Create new segment file
	segmentPath := filepath.Join(s.dataDir, fmt.Sprintf("commitlog-%d.log", s.segmentID))
	file, err := os.OpenFile(segmentPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open commit log file: %w", err)
	}

	s.currentFile = file
	s.segmentID = time.Now().Unix()

	s.logger.Info("Opened new commit log segment", zap.String("path", segmentPath))

	return nil
}

// rotationChecker periodically checks if rotation is needed
func (s *CommitLogService) rotationChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkRotation()
		case <-s.stopChan:
			return
		}
	}
}

// checkRotation checks if commit log needs rotation
func (s *CommitLogService) checkRotation() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentFile == nil {
		return
	}

	// Check file size
	fileInfo, err := s.currentFile.Stat()
	if err != nil {
		s.logger.Error("Failed to stat commit log", zap.Error(err))
		return
	}

	if fileInfo.Size() >= s.config.SegmentSize {
		s.logger.Info("Rotating commit log due to size",
			zap.Int64("size", fileInfo.Size()),
			zap.Int64("threshold", s.config.SegmentSize))

		if err := s.openNewSegment(); err != nil {
			s.logger.Error("Failed to rotate commit log", zap.Error(err))
		}
	}
}

// Recover replays commit log entries on startup with checksum validation
func (s *CommitLogService) Recover(ctx context.Context, memTableSvc *MemTableService) error {
	s.logger.Info("Starting commit log recovery")

	// Find all commit log files
	files, err := filepath.Glob(filepath.Join(s.dataDir, "commitlog-*.log"))
	if err != nil {
		return fmt.Errorf("failed to list commit log files: %w", err)
	}

	// Sort files by timestamp (embedded in filename) to ensure ordered replay
	sort.Strings(files)

	recovered := 0
	skipped := 0
	maxSeqNum := uint64(0)

	for _, filePath := range files {
		count, skipCount, maxSeq, err := s.recoverFromFile(ctx, filePath, memTableSvc)
		if err != nil {
			s.logger.Error("Failed to recover from file",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}
		recovered += count
		skipped += skipCount
		if maxSeq > maxSeqNum {
			maxSeqNum = maxSeq
		}
	}

	// Update sequence number to continue from where we left off
	atomic.StoreUint64(&s.sequenceNumber, maxSeqNum)

	s.logger.Info("Commit log recovery completed",
		zap.Int("recovered", recovered),
		zap.Int("skipped_corrupted", skipped),
		zap.Uint64("max_sequence", maxSeqNum))
	return nil
}

// recoverFromFile recovers entries from a single commit log file with checksum validation
// Returns: (recovered_count, skipped_count, max_sequence_number, error)
func (s *CommitLogService) recoverFromFile(
	ctx context.Context,
	filePath string,
	memTableSvc *MemTableService,
) (int, int, uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer size to handle large entries
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	count := 0
	skipped := 0
	maxSeqNum := uint64(0)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry model.CommitLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			s.logger.Warn("Failed to unmarshal commit log entry, skipping",
				zap.String("file", filePath),
				zap.Error(err))
			skipped++
			continue
		}

		// Validate checksum if present
		if entry.Checksum != 0 {
			// Re-serialize without checksum to validate
			originalChecksum := entry.Checksum
			entry.Checksum = 0
			data, err := json.Marshal(entry)
			if err != nil {
				s.logger.Warn("Failed to marshal entry for checksum validation",
					zap.String("file", filePath),
					zap.Error(err))
				skipped++
				continue
			}

			if !util.ValidateChecksum(data, originalChecksum) {
				s.logger.Warn("Checksum validation failed, skipping corrupted entry",
					zap.String("file", filePath),
					zap.Uint64("sequence", entry.SequenceNumber),
					zap.Uint32("expected", originalChecksum),
					zap.Uint32("actual", util.ComputeChecksum(data)))
				skipped++
				continue
			}

			// Restore checksum for processing
			entry.Checksum = originalChecksum
		}

		// Track max sequence number
		if entry.SequenceNumber > maxSeqNum {
			maxSeqNum = entry.SequenceNumber
		}

		// Replay entry to memtable
		compositeKey := fmt.Sprintf("%s:%s", entry.TenantID, entry.Key)
		memEntry := &model.MemTableEntry{
			Key:         compositeKey,
			Value:       entry.Value,
			VectorClock: entry.VectorClock,
			Timestamp:   entry.Timestamp,
			IsTombstone: entry.OperationType == model.OperationTypeDelete,
		}

		if err := memTableSvc.Put(ctx, memEntry); err != nil {
			s.logger.Warn("Failed to replay entry to memtable",
				zap.String("file", filePath),
				zap.Uint64("sequence", entry.SequenceNumber),
				zap.Error(err))
			skipped++
			continue
		}

		count++
	}

	if err := scanner.Err(); err != nil {
		return count, skipped, maxSeqNum, err
	}

	s.logger.Info("Recovered from commit log file",
		zap.String("file", filePath),
		zap.Int("recovered", count),
		zap.Int("skipped", skipped))

	return count, skipped, maxSeqNum, nil
}

// Close closes the commit log service
func (s *CommitLogService) Close() error {
	close(s.stopChan)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentFile != nil {
		return s.currentFile.Close()
	}
	return nil
}
