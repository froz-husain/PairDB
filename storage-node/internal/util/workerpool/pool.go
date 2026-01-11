package workerpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Task represents a unit of work to be executed
type Task struct {
	ID      string
	Fn      func(context.Context) error
	Context context.Context
}

// WorkerPool manages a bounded pool of goroutines for executing tasks
type WorkerPool struct {
	name           string
	maxWorkers     int
	taskQueue      chan Task
	queueSize      int
	logger         *zap.Logger
	wg             sync.WaitGroup
	stopOnce       sync.Once
	stopChan       chan struct{}
	activeWorkers  int32
	totalTasks     uint64
	completedTasks uint64
	failedTasks    uint64
	rejectedTasks  uint64
}

// Config holds worker pool configuration
type Config struct {
	Name         string
	MaxWorkers   int
	QueueSize    int
	Logger       *zap.Logger
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(cfg *Config) *WorkerPool {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 10
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	pool := &WorkerPool{
		name:       cfg.Name,
		maxWorkers: cfg.MaxWorkers,
		queueSize:  cfg.QueueSize,
		taskQueue:  make(chan Task, cfg.QueueSize),
		logger:     cfg.Logger,
		stopChan:   make(chan struct{}),
	}

	// Start workers
	for i := 0; i < pool.maxWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	pool.logger.Info("Worker pool started",
		zap.String("name", pool.name),
		zap.Int("max_workers", pool.maxWorkers),
		zap.Int("queue_size", pool.queueSize))

	return pool
}

// worker is the main worker goroutine
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	p.logger.Debug("Worker started",
		zap.String("pool", p.name),
		zap.Int("worker_id", id))

	for {
		select {
		case <-p.stopChan:
			p.logger.Debug("Worker stopping",
				zap.String("pool", p.name),
				zap.Int("worker_id", id))
			return

		case task := <-p.taskQueue:
			p.executeTask(id, task)
		}
	}
}

// executeTask executes a single task
func (p *WorkerPool) executeTask(workerID int, task Task) {
	atomic.AddInt32(&p.activeWorkers, 1)
	defer atomic.AddInt32(&p.activeWorkers, -1)

	start := time.Now()

	// Execute task with panic recovery
	err := p.safeExecute(task)

	duration := time.Since(start)

	if err != nil {
		atomic.AddUint64(&p.failedTasks, 1)
		p.logger.Error("Task failed",
			zap.String("pool", p.name),
			zap.Int("worker_id", workerID),
			zap.String("task_id", task.ID),
			zap.Duration("duration", duration),
			zap.Error(err))
	} else {
		atomic.AddUint64(&p.completedTasks, 1)
		p.logger.Debug("Task completed",
			zap.String("pool", p.name),
			zap.Int("worker_id", workerID),
			zap.String("task_id", task.ID),
			zap.Duration("duration", duration))
	}
}

// safeExecute executes a task with panic recovery
func (p *WorkerPool) safeExecute(task Task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("task panicked: %v", r)
			p.logger.Error("Task panic recovered",
				zap.String("pool", p.name),
				zap.String("task_id", task.ID),
				zap.Any("panic", r))
		}
	}()

	if task.Context == nil {
		task.Context = context.Background()
	}

	return task.Fn(task.Context)
}

// Submit submits a task to the worker pool
// Returns error if the queue is full or pool is stopped
func (p *WorkerPool) Submit(task Task) error {
	select {
	case <-p.stopChan:
		atomic.AddUint64(&p.rejectedTasks, 1)
		return fmt.Errorf("worker pool '%s' is stopped", p.name)
	default:
	}

	select {
	case p.taskQueue <- task:
		atomic.AddUint64(&p.totalTasks, 1)
		return nil
	default:
		atomic.AddUint64(&p.rejectedTasks, 1)
		return fmt.Errorf("worker pool '%s' queue is full", p.name)
	}
}

// SubmitWithContext submits a task with context and blocks until accepted or context canceled
func (p *WorkerPool) SubmitWithContext(ctx context.Context, task Task) error {
	select {
	case <-p.stopChan:
		atomic.AddUint64(&p.rejectedTasks, 1)
		return fmt.Errorf("worker pool '%s' is stopped", p.name)
	case <-ctx.Done():
		atomic.AddUint64(&p.rejectedTasks, 1)
		return ctx.Err()
	case p.taskQueue <- task:
		atomic.AddUint64(&p.totalTasks, 1)
		return nil
	}
}

// TrySubmit attempts to submit a task without blocking
// Returns false if queue is full or pool is stopped
func (p *WorkerPool) TrySubmit(task Task) bool {
	select {
	case <-p.stopChan:
		atomic.AddUint64(&p.rejectedTasks, 1)
		return false
	case p.taskQueue <- task:
		atomic.AddUint64(&p.totalTasks, 1)
		return true
	default:
		atomic.AddUint64(&p.rejectedTasks, 1)
		return false
	}
}

// Stop gracefully stops the worker pool
// Waits for all workers to finish their current tasks
func (p *WorkerPool) Stop(timeout time.Duration) error {
	var err error
	p.stopOnce.Do(func() {
		p.logger.Info("Stopping worker pool", zap.String("name", p.name))
		close(p.stopChan)

		// Wait for workers with timeout
		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			p.logger.Info("Worker pool stopped gracefully", zap.String("name", p.name))
		case <-time.After(timeout):
			err = fmt.Errorf("worker pool '%s' stop timeout after %v", p.name, timeout)
			p.logger.Warn("Worker pool stop timeout", zap.String("name", p.name))
		}
	})
	return err
}

// Stats returns current worker pool statistics
func (p *WorkerPool) Stats() Stats {
	return Stats{
		Name:           p.name,
		MaxWorkers:     p.maxWorkers,
		ActiveWorkers:  int(atomic.LoadInt32(&p.activeWorkers)),
		QueueSize:      p.queueSize,
		QueuedTasks:    len(p.taskQueue),
		TotalTasks:     atomic.LoadUint64(&p.totalTasks),
		CompletedTasks: atomic.LoadUint64(&p.completedTasks),
		FailedTasks:    atomic.LoadUint64(&p.failedTasks),
		RejectedTasks:  atomic.LoadUint64(&p.rejectedTasks),
	}
}

// Stats represents worker pool statistics
type Stats struct {
	Name           string
	MaxWorkers     int
	ActiveWorkers  int
	QueueSize      int
	QueuedTasks    int
	TotalTasks     uint64
	CompletedTasks uint64
	FailedTasks    uint64
	RejectedTasks  uint64
}

// QueueUtilization returns the queue utilization as a percentage
func (s Stats) QueueUtilization() float64 {
	if s.QueueSize == 0 {
		return 0
	}
	return (float64(s.QueuedTasks) / float64(s.QueueSize)) * 100.0
}

// WorkerUtilization returns the worker utilization as a percentage
func (s Stats) WorkerUtilization() float64 {
	if s.MaxWorkers == 0 {
		return 0
	}
	return (float64(s.ActiveWorkers) / float64(s.MaxWorkers)) * 100.0
}

// SuccessRate returns the task success rate as a percentage
func (s Stats) SuccessRate() float64 {
	if s.TotalTasks == 0 {
		return 100.0
	}
	return (float64(s.CompletedTasks) / float64(s.TotalTasks)) * 100.0
}
