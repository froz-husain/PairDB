package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// CommitLogService manages write-ahead logging for durability
type CommitLogService struct {
	config      *CommitLogConfig
	currentFile *os.File
	logger      *zap.Logger
	mu          sync.Mutex
	dataDir     string
	segmentID   int64
	stopChan    chan struct{}
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

// Append appends an entry to the commit log
func (s *CommitLogService) Append(ctx context.Context, entry *model.CommitLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize entry
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
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

// Recover replays commit log entries on startup
func (s *CommitLogService) Recover(ctx context.Context, memTableSvc *MemTableService) error {
	s.logger.Info("Starting commit log recovery")

	// Find all commit log files
	files, err := filepath.Glob(filepath.Join(s.dataDir, "commitlog-*.log"))
	if err != nil {
		return fmt.Errorf("failed to list commit log files: %w", err)
	}

	recovered := 0
	for _, filePath := range files {
		count, err := s.recoverFromFile(ctx, filePath, memTableSvc)
		if err != nil {
			s.logger.Error("Failed to recover from file",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}
		recovered += count
	}

	s.logger.Info("Commit log recovery completed", zap.Int("entries", recovered))
	return nil
}

// recoverFromFile recovers entries from a single commit log file
func (s *CommitLogService) recoverFromFile(
	ctx context.Context,
	filePath string,
	memTableSvc *MemTableService,
) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		var entry model.CommitLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			s.logger.Warn("Failed to unmarshal commit log entry", zap.Error(err))
			continue
		}

		// Replay entry to memtable
		compositeKey := fmt.Sprintf("%s:%s", entry.TenantID, entry.Key)
		memEntry := &model.MemTableEntry{
			Key:         compositeKey,
			Value:       entry.Value,
			VectorClock: entry.VectorClock,
			Timestamp:   entry.Timestamp,
		}

		if err := memTableSvc.Put(ctx, memEntry); err != nil {
			s.logger.Warn("Failed to replay entry to memtable", zap.Error(err))
			continue
		}

		count++
	}

	if err := scanner.Err(); err != nil {
		return count, err
	}

	return count, nil
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
