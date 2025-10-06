package utils

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

// Pool[T] is a worker pool backed by WorkerPoolQueue.
// It uses an atomic inflight counter + cond to avoid deadlocks between closing the queue
// and tracking outstanding work.
type Pool[T any] struct {
	tasks       *WorkerPoolQueue[T]
	workerCount int

	ctx    context.Context
	cancel context.CancelFunc

	workersWg sync.WaitGroup // wait for worker goroutines to exit

	closeOnce sync.Once

	firstErr atomic.Pointer[error]

	closedForSubmit atomic.Bool

	// inflight counter and condition for waiting until work drains
	inflight   int64
	inflightMu sync.Mutex
	inflightC  *sync.Cond
}

// NewWorkerPool creates a new Pool. If workerCount <= 0 it defaults to runtime.NumCPU().
func NewWorkerPool[T any](parent context.Context, workerCount int) *Pool[T] {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	ctx, cancel := context.WithCancel(parent)
	p := &Pool[T]{
		tasks:       NewWorkerPoolQueue[T](),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}
	p.inflightC = sync.NewCond(&p.inflightMu)
	return p
}

// addInflight increments inflight and must be called when a task is known submitted.
func (p *Pool[T]) addInflight(delta int64) {
	atomic.AddInt64(&p.inflight, delta)
	if atomic.LoadInt64(&p.inflight) == 0 {
		// wake any waiter (lock to satisfy cond's invariant)
		p.inflightMu.Lock()
		p.inflightC.Broadcast()
		p.inflightMu.Unlock()
	}
}

// Submit enqueues a task. It increments the inflight counter BEFORE attempting to enqueue.
// If ctx is already cancelled, Submit returns ctx.Err() and does NOT increment inflight.
func (p *Pool[T]) Submit(item T) error {
	// fast-fail if canceled
	if err := p.ctx.Err(); err != nil {
		return err
	}

	// increment inflight first
	p.addInflight(1)

	// try to put into queue
	if err := p.tasks.Put(item); err != nil {
		// queue closed (or otherwise failed) -> unaccount the inflight and wake waiters if needed
		p.addInflight(-1)
		return err
	}
	return nil
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
			for {
				item, ok := p.tasks.Get()
				if !ok {
					// queue closed and drained -> exit worker
					return
				}

				// If ctx canceled, we must still decrement inflight for this item and skip handler.
				if p.ctx.Err() != nil {
					p.addInflight(-1)
					continue
				}

				// run handler (handler may call p.Submit)
				if err := handler(p.ctx, item, func(it T) error { return p.Submit(it) }); err != nil {
					// store first error safely (allocate on heap)
					ep := new(error)
					*ep = err
					p.firstErr.CompareAndSwap(nil, ep)

					// cancel pool so other workers see ctx canceled
					p.cancel()

					// decrement inflight for this item
					p.addInflight(-1)

					// force-close the queue and abandon queued items (so we won't wait forever)
					p.forceClose()

					// continue so other workers can observe ctx and drain/exit
					continue
				}

				// normal completion of this task: decrement inflight
				p.addInflight(-1)
			}
		}()
	}

	// monitor goroutine: when CloseForSubmit has been called, wait until both inflight==0 and queue empty,
	// then close the queue so workers exit. Also handle ctx cancellation (force-close).
	go func() {
		for {
			// graceful path: wait for CloseForSubmit flag then wait for work to drain
			if p.closedForSubmit.Load() {
				// wait until inflight==0 AND tasks.Len()==0
				p.inflightMu.Lock()
				for {
					if atomic.LoadInt64(&p.inflight) == 0 && p.tasks.Len() == 0 {
						break
					}
					p.inflightC.Wait()
					// loop and re-check
				}
				p.inflightMu.Unlock()

				// Now safe to close queue: there is no inflight and no queued items
				p.closeOnce.Do(func() { p.tasks.Close() })
				return
			}

			// if ctx canceled -> force-close path
			if p.ctx.Err() != nil {
				p.forceClose()
				return
			}

			// avoid busy spin
			runtime.Gosched()
		}
	}()
}

// CloseForSubmit indicates the caller will not submit more external (caller-side) tasks.
// Workers may still call Submit to add child tasks. When inflight reaches zero and queue is empty,
// the pool will close tasks so workers exit.
func (p *Pool[T]) CloseForSubmit() {
	p.closedForSubmit.Store(true)
	// kick the monitor by signaling condition in case inflight==0 already
	p.inflightMu.Lock()
	p.inflightC.Broadcast()
	p.inflightMu.Unlock()
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

// forceClose performs a one-time forced shutdown: cancel context, close queue and
// subtract any queued-but-unprocessed items from inflight so waiters don't block forever.
func (p *Pool[T]) forceClose() {
	p.cancel()
	p.closeOnce.Do(func() {
		// first capture queued items
		queued := p.tasks.Len()
		if queued > 0 {
			// reduce inflight by queued. Use atomic and then broadcast condition.
			// Ensure we don't go negative.
			for {
				cur := atomic.LoadInt64(&p.inflight)
				// clamp
				var toSub int64 = int64(queued)
				if toSub > cur {
					toSub = cur
				}
				if toSub == 0 {
					break
				}
				if atomic.CompareAndSwapInt64(&p.inflight, cur, cur-toSub) {
					p.inflightMu.Lock()
					p.inflightC.Broadcast()
					p.inflightMu.Unlock()
					break
				}
				// retry on CAS failure
			}
		}
		// now close the queue to wake Get() waiters
		p.tasks.Close()
	})
}

var ErrClosed = errors.New("queue closed")

type node[T any] struct {
	val  T
	next atomic.Pointer[node[T]]
}

// WorkerPoolQueue is a dual-lock MPMC linked-list queue.
// Node.next is an atomic.Pointer to avoid races between producers (writing next)
// and consumers (reading next) when they hold different locks.
type WorkerPoolQueue[T any] struct {
	head   *node[T]    // sentinel
	tail   *node[T]    // last node
	headMu sync.Mutex  // protects head and cond / waiting
	tailMu sync.Mutex  // protects tail pointer
	cond   *sync.Cond  // tied to headMu
	closed atomic.Bool // closed flag (atomic)
	size   int64       // approximate size
}

func NewWorkerPoolQueue[T any]() *WorkerPoolQueue[T] {
	s := &node[T]{}
	q := &WorkerPoolQueue[T]{head: s, tail: s}
	q.cond = sync.NewCond(&q.headMu)
	return q
}

func (q *WorkerPoolQueue[T]) Put(v T) error {
	// Quick closed check
	if q.closed.Load() {
		return ErrClosed
	}

	n := &node[T]{}
	n.val = v

	for {
		// Fast path: append under tailMu if queue is non-empty.
		q.tailMu.Lock()
		if q.closed.Load() {
			q.tailMu.Unlock()
			return ErrClosed
		}
		if q.tail != q.head { // non-empty
			// use atomic store for tail.next
			q.tail.next.Store(n)
			q.tail = n
			atomic.AddInt64(&q.size, 1)
			q.tailMu.Unlock()
			// wake a waiter (ok to call Signal without headMu)
			q.cond.Signal()
			return nil
		}
		// maybe empty: release tailMu and take both locks in canonical order
		q.tailMu.Unlock()

		q.headMu.Lock()
		q.tailMu.Lock()

		// re-check closed & emptiness
		if q.closed.Load() {
			q.tailMu.Unlock()
			q.headMu.Unlock()
			return ErrClosed
		}
		if q.tail != q.head {
			// someone appended in the meantime: release and retry
			q.tailMu.Unlock()
			q.headMu.Unlock()
			continue
		}

		// safe to append: we hold headMu+tailMu and queue is empty
		q.tail.next.Store(n)
		q.tail = n
		atomic.AddInt64(&q.size, 1)
		// signal while holding headMu (canonical)
		q.cond.Signal()
		q.tailMu.Unlock()
		q.headMu.Unlock()
		return nil
	}
}

func (q *WorkerPoolQueue[T]) Get() (T, bool) {
	q.headMu.Lock()
	// wait while empty and not closed
	for q.head.next.Load() == nil && !q.closed.Load() {
		q.cond.Wait()
	}

	// empty + closed => done
	if q.head.next.Load() == nil {
		q.headMu.Unlock()
		var zero T
		return zero, false
	}

	// pop head.next (atomic load)
	n := q.head.next.Load()
	// set head.next = n.next (atomic load of next)
	next := n.next.Load()
	q.head.next.Store(next)

	// If queue became empty after pop, reset tail -> head sentinel if safe.
	if q.head.next.Load() == nil {
		q.tailMu.Lock()
		// compare pointer identity under tailMu
		if q.tail == n {
			q.tail = q.head
		}
		q.tailMu.Unlock()
	}

	q.headMu.Unlock()
	atomic.AddInt64(&q.size, -1)
	return n.val, true
}

func (q *WorkerPoolQueue[T]) TryGet() (T, bool) {
	q.headMu.Lock()
	if q.head.next.Load() == nil {
		q.headMu.Unlock()
		var zero T
		return zero, false
	}
	n := q.head.next.Load()
	next := n.next.Load()
	q.head.next.Store(next)
	if q.head.next.Load() == nil {
		q.tailMu.Lock()
		if q.tail == n {
			q.tail = q.head
		}
		q.tailMu.Unlock()
	}
	q.headMu.Unlock()
	atomic.AddInt64(&q.size, -1)
	return n.val, true
}

func (q *WorkerPoolQueue[T]) Len() int {
	return int(atomic.LoadInt64(&q.size))
}

func (q *WorkerPoolQueue[T]) Close() {
	q.closed.Store(true)
	q.headMu.Lock()
	q.cond.Broadcast()
	q.headMu.Unlock()
}
