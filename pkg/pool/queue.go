package pool

import (
	"sync"
	"sync/atomic"
)

// noCopy may be embedded into structs which must not be copied after first use.
// go vet will warn on accidental copies (it looks for Lock methods).
type noCopy struct{}

func (*noCopy) Lock() {}

// node for single-lock queue (plain pointer; protected by mu)
type node[T any] struct {
	val  T
	next *node[T]
}

// WorkerPoolQueue is a simple, single-mutex MPMC queue.
// This is easier to reason about than a two-lock variant and avoids lost-wakeup races.
type WorkerPoolQueue[T any] struct {
	noCopy noCopy

	mu     sync.Mutex
	cond   *sync.Cond
	head   *node[T] // sentinel
	tail   *node[T]
	closed bool
	size   int64 // track queued count (atomic operations used for Len to avoid taking mu)
}

// NewWorkerPoolQueue constructs a new queue.
func NewWorkerPoolQueue[T any]() *WorkerPoolQueue[T] {
	s := &node[T]{}
	q := &WorkerPoolQueue[T]{head: s, tail: s}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *WorkerPoolQueue[T]) Put(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	n := &node[T]{val: v}
	q.tail.next = n
	q.tail = n
	atomic.AddInt64(&q.size, 1)
	// signal one waiter (consumer checks under mu)
	q.cond.Signal()
	return nil
}

func (q *WorkerPoolQueue[T]) Get() (T, bool) {
	q.mu.Lock()
	// wait while empty and not closed
	for q.head.next == nil && !q.closed {
		q.cond.Wait()
	}

	// empty + closed => done
	if q.head.next == nil {
		q.mu.Unlock()
		var zero T
		return zero, false
	}

	// pop head.next
	n := q.head.next
	q.head.next = n.next
	if q.head.next == nil {
		q.tail = q.head
	}
	q.mu.Unlock()

	atomic.AddInt64(&q.size, -1)
	return n.val, true
}

func (q *WorkerPoolQueue[T]) TryGet() (T, bool) {
	q.mu.Lock()
	if q.head.next == nil {
		q.mu.Unlock()
		var zero T
		return zero, false
	}
	n := q.head.next
	q.head.next = n.next
	if q.head.next == nil {
		q.tail = q.head
	}
	q.mu.Unlock()
	atomic.AddInt64(&q.size, -1)
	return n.val, true
}

func (q *WorkerPoolQueue[T]) Len() int {
	return int(atomic.LoadInt64(&q.size))
}

func (q *WorkerPoolQueue[T]) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
