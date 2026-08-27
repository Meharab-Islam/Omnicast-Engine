package signaling

import (
	"sync"
)

// WorkerPool manages a fixed set of background worker goroutines for concurrent broadcasting and media operations
type WorkerPool struct {
	tasks    chan func()
	capacity int
	wg       sync.WaitGroup
	quit     chan struct{}
	once     sync.Once
}

// NewWorkerPool creates and starts a WorkerPool with the given number of worker goroutines and task queue capacity
func NewWorkerPool(workers, queueCap int) *WorkerPool {
	if workers <= 0 {
		workers = 32
	}
	if queueCap <= 0 {
		queueCap = 16384
	}

	pool := &WorkerPool{
		tasks:    make(chan func(), queueCap),
		capacity: workers,
		quit:     make(chan struct{}),
	}

	pool.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer pool.wg.Done()
			for {
				select {
				case task, ok := <-pool.tasks:
					if !ok {
						return
					}
					task()
				case <-pool.quit:
					return
				}
			}
		}()
	}

	return pool
}

// Submit dispatches a task to the worker pool. If the queue is saturated, it executes synchronously as fallback.
func (p *WorkerPool) Submit(task func()) {
	if task == nil {
		return
	}
	select {
	case p.tasks <- task:
	default:
		// Queue saturated: run in background goroutine fallback
		go task()
	}
}

// Stop gracefully shuts down the worker pool
func (p *WorkerPool) Stop() {
	p.once.Do(func() {
		close(p.quit)
		close(p.tasks)
		p.wg.Wait()
	})
}
