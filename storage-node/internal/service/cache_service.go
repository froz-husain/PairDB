package service

import (
	"sync"
	"time"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"go.uber.org/zap"
)

// CacheService implements an adaptive LRU/LFU cache
type CacheService struct {
	config          *CacheConfig
	cache           map[string]*model.CacheEntry
	logger          *zap.Logger
	mu              sync.RWMutex
	currentSize     int64
	frequencyWeight float64
	recencyWeight   float64
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	MaxSize         int64
	FrequencyWeight float64
	RecencyWeight   float64
	AdaptiveWindow  time.Duration
}

// NewCacheService creates a new cache service
func NewCacheService(cfg *CacheConfig, logger *zap.Logger) *CacheService {
	return &CacheService{
		config:          cfg,
		cache:           make(map[string]*model.CacheEntry),
		logger:          logger,
		frequencyWeight: cfg.FrequencyWeight,
		recencyWeight:   cfg.RecencyWeight,
	}
}

// Get retrieves a value from cache
func (s *CacheService) Get(key string) (*model.CacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, found := s.cache[key]
	if !found {
		return nil, false
	}

	// Update access statistics
	entry.AccessCount++
	entry.LastAccess = time.Now()
	entry.Score = s.calculateScore(entry)

	return entry, true
}

// Put adds or updates a value in cache
func (s *CacheService) Put(key string, value []byte, vectorClock model.VectorClock) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entrySize := int64(len(key) + len(value) + 64)

	// Check if key already exists
	if existing, found := s.cache[key]; found {
		// Update existing entry
		oldSize := int64(len(existing.Key) + len(existing.Value) + 64)
		existing.Value = value
		existing.VectorClock = vectorClock
		existing.AccessCount++
		existing.LastAccess = time.Now()
		existing.Score = s.calculateScore(existing)
		s.currentSize = s.currentSize - oldSize + entrySize
		return
	}

	// Check if cache is full
	for s.currentSize+entrySize > s.config.MaxSize {
		s.evictLowestScore()
	}

	// Add new entry
	entry := &model.CacheEntry{
		Key:         key,
		Value:       value,
		VectorClock: vectorClock,
		AccessCount: 1,
		LastAccess:  time.Now(),
	}
	entry.Score = s.calculateScore(entry)

	s.cache[key] = entry
	s.currentSize += entrySize
}

// Remove removes a key from cache
func (s *CacheService) Remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, found := s.cache[key]; found {
		entrySize := int64(len(entry.Key) + len(entry.Value) + 64)
		delete(s.cache, key)
		s.currentSize -= entrySize
	}
}

// calculateScore computes adaptive score for eviction
func (s *CacheService) calculateScore(entry *model.CacheEntry) float64 {
	// Frequency component (normalized)
	frequencyScore := float64(entry.AccessCount)

	// Recency component (time since last access in seconds)
	recencyScore := time.Since(entry.LastAccess).Seconds()

	// Combined adaptive score (higher is better)
	score := s.frequencyWeight*frequencyScore - s.recencyWeight*recencyScore

	return score
}

// evictLowestScore evicts the entry with lowest score
func (s *CacheService) evictLowestScore() {
	var lowestKey string
	var lowestScore float64 = 1e9

	for key, entry := range s.cache {
		if entry.Score < lowestScore {
			lowestScore = entry.Score
			lowestKey = key
		}
	}

	if lowestKey != "" {
		entry := s.cache[lowestKey]
		entrySize := int64(len(entry.Key) + len(entry.Value) + 64)

		delete(s.cache, lowestKey)
		s.currentSize -= entrySize

		s.logger.Debug("Evicted cache entry",
			zap.String("key", lowestKey),
			zap.Float64("score", lowestScore))
	}
}

// AdjustWeights adjusts frequency and recency weights based on workload
func (s *CacheService) AdjustWeights() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Analyze access patterns
	var totalAccesses int64
	var recentAccesses int64
	recentThreshold := time.Now().Add(-s.config.AdaptiveWindow)

	for _, entry := range s.cache {
		totalAccesses += entry.AccessCount
		if entry.LastAccess.After(recentThreshold) {
			recentAccesses++
		}
	}

	if totalAccesses == 0 {
		return
	}

	// Calculate hotness ratio
	hotnessRatio := float64(recentAccesses) / float64(len(s.cache))

	// Adjust weights based on hotness
	if hotnessRatio > 0.7 {
		// High recency workload - favor LRU
		s.recencyWeight = 0.7
		s.frequencyWeight = 0.3
	} else if hotnessRatio < 0.3 {
		// High frequency workload - favor LFU
		s.recencyWeight = 0.3
		s.frequencyWeight = 0.7
	} else {
		// Balanced workload
		s.recencyWeight = 0.5
		s.frequencyWeight = 0.5
	}

	s.logger.Debug("Adjusted cache weights",
		zap.Float64("recency_weight", s.recencyWeight),
		zap.Float64("frequency_weight", s.frequencyWeight),
		zap.Float64("hotness_ratio", hotnessRatio))
}

// Stats returns cache statistics
func (s *CacheService) Stats() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return CacheStats{
		Size:         s.currentSize,
		MaxSize:      s.config.MaxSize,
		EntryCount:   len(s.cache),
		UsagePercent: float64(s.currentSize) / float64(s.config.MaxSize) * 100,
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	Size         int64
	MaxSize      int64
	EntryCount   int
	UsagePercent float64
}
