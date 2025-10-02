package utils

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

// Pool[T] is a channel-driven worker pool that tracks pending tasks via a WaitGroup.
// Submit increments the pending WaitGroup before enqueuing; workers call pending.Done()
// after processing a task. Caller must call CloseForSubmit() once when external seeding is finished.
type Pool[T any] struct {
	tasks       chan T
	workerCount int

	ctx    context.Context
	cancel context.CancelFunc

	workersWg sync.WaitGroup // wait for worker goroutines to exit
	pending   sync.WaitGroup // counts submitted-but-not-done tasks

	closeOnce sync.Once

	firstErr atomic.Pointer[error]

	closedForSubmit atomic.Bool
}

// NewWorkerPool creates a new Pool. If workerCount <= 0 it defaults to runtime.NumCPU().
// buf is the internal task-channel buffer size (if <=0 defaults to workerCount).
func NewWorkerPool[T any](parent context.Context, workerCount, buf int) *Pool[T] {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if buf <= 0 {
		buf = workerCount
	}
	ctx, cancel := context.WithCancel(parent)
	return &Pool[T]{
		tasks:       make(chan T, buf),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Submit enqueues a task. It increments the pending WaitGroup BEFORE attempting to send.
// If ctx is already cancelled, Submit returns ctx.Err() and does NOT increment pending.
func (p *Pool[T]) Submit(item T) error {
	// fast-fail if canceled
	if p.ctx.Err() != nil {
		return p.ctx.Err()
	}

	// account for this task first
	p.pending.Add(1)

	// try to send; if ctx cancelled while sending, balance the pending counter
	select {
	case p.tasks <- item:
		return nil
	case <-p.ctx.Done():
		p.pending.Done() // balance
		return p.ctx.Err()
	}

}

// Start spawns workerCount workers that call handler(ctx, item, submit).
// Handler should process the item and return an error if it wants to abort the whole pool.
// Handler may call submit(...) to add child tasks (workers are allowed to submit).
func (p *Pool[T]) Start(handler func(ctx context.Context, item T, submit func(T) error) error) {
	// spawn workers
	p.workersWg.Add(p.workerCount)
	for i := 0; i < p.workerCount; i++ {
		go func() {
			defer p.workersWg.Done()
			for item := range p.tasks {
				// if canceled, mark task done and continue so pending can reach zero and close can proceed
				if p.ctx.Err() != nil {
					p.pending.Done()
					continue
				}
				// run handler (handler may call p.Submit)
				if err := handler(p.ctx, item, func(it T) error { return p.Submit(it) }); err != nil {
					// store first error and cancel
					e := err
					p.firstErr.CompareAndSwap(nil, &e)
					p.cancel()
					// still mark this task done
					p.pending.Done()
					// continue draining (workers will see ctx canceled and just Done() remaining items)
					continue
				}
				// normal completion of this task
				p.pending.Done()
			}
		}()
	}

	// close the tasks channel once pending is zero and CloseForSubmit() has been called
	go func() {
		// Wait until CloseForSubmit is called (external seeding done)
		// After CloseForSubmit, this goroutine waits for pending.Wait() to reach zero,
		// then closes the tasks channel exactly once so workers can exit.
		for !p.closedForSubmit.Load() {
			// spin-wait/check; small sleep could be added but not necessary if CloseForSubmit will be called soon
			// We could also use a conditional variable, but this is sufficient and simple.
			// Alternatively, CloseForSubmit could spawn the pending-wait goroutine directly (done below).
			// Here, just yield
			if p.ctx.Err() != nil {
				// aborted externally; close tasks to let workers exit
				p.closeOnce.Do(func() { close(p.tasks) })
				return
			}
		}

		// when CloseForSubmit has been called, wait for pending to drain
		p.pending.Wait()
		p.closeOnce.Do(func() { close(p.tasks) })
	}()
}

// CloseForSubmit indicates the caller will not submit more external (caller-side) tasks.
// Workers may still call Submit to add child tasks. When pending reaches zero, the pool
// closes the tasks channel to stop workers.
func (p *Pool[T]) CloseForSubmit() {
	p.closedForSubmit.Store(true)
	// if there are already no pending tasks, close immediately
	go func() {
		p.pending.Wait()
		p.closeOnce.Do(func() { close(p.tasks) })
	}()
}

// Wait blocks until all workers have exited and returns the first error (if any).
func (p *Pool[T]) Wait() error {
	p.workersWg.Wait()
	if e := p.firstErr.Load(); e != nil && *e != nil {
		return *e
	}
	if p.ctx.Err() != nil && !errors.Is(p.ctx.Err(), context.Canceled) {
		return p.ctx.Err()
	}
	return nil
}

// Close cancels the pool and closes the internal tasks channel (force-stop).
func (p *Pool[T]) Close() {
	p.cancel()
	p.closeOnce.Do(func() { close(p.tasks) })
}
