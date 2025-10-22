// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// --- Task helpers (lightweight) ---

// Task is a unit of work executed by the shared worker pool.
// submit allows spawning child tasks into the same logical/virtual pool.
type Task interface {
	Run(ctx context.Context, submit func(Task) error) error
}

// TaskFunc convenience adapter so closures are easy to submit.
type TaskFunc func(ctx context.Context, submit func(Task) error) error

func (f TaskFunc) Run(ctx context.Context, submit func(Task) error) error {
	if f == nil {
		return nil
	}
	return f(ctx, submit)
}

// --- ErrorCollector (per-virtual tolerant mode) ---

// ErrorCollector collects errors for a virtual pool.
// It stores a snapshotable slice and provides a live channel for streaming.
type ErrorCollector struct {
	mu   sync.Mutex
	errs []error
	Ch   chan error
}

func newErrorCollector(buf int) *ErrorCollector {
	if buf <= 0 {
		buf = 1024
	}
	return &ErrorCollector{
		Ch: make(chan error, buf),
	}
}

func (ec *ErrorCollector) add(err error) {
	if err == nil {
		return
	}
	ec.mu.Lock()
	ec.errs = append(ec.errs, err)
	ec.mu.Unlock()

	select {
	case ec.Ch <- err:
	default:
		// drop if full
	}
}

// Errors returns a snapshot of collected errors.
func (ec *ErrorCollector) Errors() []error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	out := make([]error, len(ec.errs))
	copy(out, ec.errs)
	return out
}

// close channel when done
func (ec *ErrorCollector) close() {
	close(ec.Ch)
}

// --- Virtual pool system ---

var ErrVirtualPoolClosed = errors.New("virtual pool closed for submit")

// VirtualMode controls virtual pool failure semantics.
type VirtualMode int

const (
	// VirtualFailFast: first error stops executing further tasks for this virtual pool.
	VirtualFailFast VirtualMode = iota
	// VirtualTolerant: errors are collected, tasks continue.
	VirtualTolerant
)

// SharedTaskPool manages a shared worker pool (reusing Pool[Task]) and provides
// creation of VirtualPools that submit into the shared pool.
type SharedTaskPool struct {
	inner *Pool[Task]

	mu   sync.RWMutex
	vmap map[string]*VirtualPool
}

// NewSharedTaskPool constructs a shared pool; caller should call Start() to begin workers.
func NewSharedTaskPool(parent context.Context, workerCount int) *SharedTaskPool {
	inner := NewWorkerPool[Task](parent, workerCount)
	// Start inner with a handler that executes Task.Run but never returns an error to
	// avoid aborting the shared pool. Per-virtual semantics are enforced by VirtualPool.
	inner.Start(func(ctx context.Context, item Task, submit func(Task) error) error {
		_ = item.Run(ctx, submit)
		return nil
	})

	return &SharedTaskPool{
		inner: inner,
		vmap:  make(map[string]*VirtualPool),
	}
}

// CloseForSubmit proxies to underlying pool when caller is done submitting to all virtuals.
func (s *SharedTaskPool) CloseForSubmit() {
	s.inner.CloseForSubmit()
}

// Wait proxies to underlying pool wait. Note: this waits for all tasks on the shared pool.
func (s *SharedTaskPool) Wait() error {
	return s.inner.Wait()
}

// NewVirtualPool creates and registers a virtual pool on top of the shared pool.
// id is an arbitrary identifier (must be unique per SharedTaskPool).
// mode controls failure semantics. buf controls error channel buffer for tolerant mode.
func (s *SharedTaskPool) NewVirtualPool(id string, mode VirtualMode, buf int) *VirtualPool {
	v := &VirtualPool{
		id:       id,
		parent:   s,
		mode:     mode,
		ec:       nil,
		closed:   atomic.Bool{},
		firstErr: atomic.Pointer[error]{},
	}
	if mode == VirtualTolerant {
		v.ec = newErrorCollector(buf)
	}
	s.mu.Lock()
	s.vmap[id] = v
	s.mu.Unlock()
	return v
}

// UnregisterVirtualPool removes the virtual pool registration (does not wait or close inner).
func (s *SharedTaskPool) UnregisterVirtualPool(id string) {
	s.mu.Lock()
	delete(s.vmap, id)
	s.mu.Unlock()
}

// submitWrapped submits a virtualTask into the shared pool.
func (s *SharedTaskPool) submitWrapped(vt *virtualTask) error {
	return s.inner.Submit(vt)
}

// --- VirtualPool types ---

// VirtualPool represents a logical pool view that reuses shared workers.
// It enforces per-virtual behaviour like fail-fast or tolerant error collection.
type VirtualPool struct {
	id     string
	parent *SharedTaskPool
	mode   VirtualMode

	ec *ErrorCollector // non-nil for VirtualTolerant

	closed atomic.Bool // closed for new submissions
	// firstErr used for fail-fast
	firstErr atomic.Pointer[error]
	// per-virtual inflight counter (matches lifecycle of tasks submitted by this virtual)
	inflight int64
	// ensure collector channel closed only once
	collectorOnce sync.Once
}

// virtualTask wraps a Task with its owning VirtualPool reference.
type virtualTask struct {
	vp   *VirtualPool
	task Task
}

func (vt *virtualTask) Run(ctx context.Context, submit func(Task) error) error {
	// If virtual is closed due to fail-fast, skip executing the task.
	if vt.vp.isFailed() {
		return nil
	}

	// build a submit wrapper so child tasks submitted by this task remain in the same virtual pool.
	submitWrapper := func(t Task) error {
		return vt.vp.Submit(t)
	}

	// Execute the actual task.
	err := vt.task.Run(ctx, submitWrapper)

	// handle per-virtual error semantics
	if err != nil {
		switch vt.vp.mode {
		case VirtualFailFast:
			vt.vp.recordFirstError(err)
			// mark closed so subsequent tasks from this virtual are skipped
			vt.vp.closed.Store(true)
		case VirtualTolerant:
			if vt.vp.ec != nil {
				vt.vp.ec.add(err)
			}
		}
	}

	// return nil to shared pool so shared pool doesn't abort
	// decrement inflight and possibly close collector if virtual is closed and no more inflight
	if remaining := atomic.AddInt64(&vt.vp.inflight, -1); remaining == 0 && vt.vp.closed.Load() {
		// close collector once
		vt.vp.collectorOnce.Do(func() {
			if vt.vp.ec != nil {
				vt.vp.ec.close()
			}
		})
	}
	return nil
}

// --- VirtualPool API ---

// Submit enqueues a Task into this virtual pool.
// It wraps the Task into a virtualTask that remembers the virtual identity.
func (v *VirtualPool) Submit(t Task) error {
	// fast-fail if virtual pool closed for submit
	if v.closed.Load() {
		return ErrVirtualPoolClosed
	}
	// If already failed (fail-fast), disallow further submissions.
	if v.isFailed() {
		return ErrVirtualPoolClosed
	}
	// increment per-virtual inflight (will be decremented by worker after run)
	atomic.AddInt64(&v.inflight, 1)
	vt := &virtualTask{vp: v, task: t}
	if err := v.parent.submitWrapped(vt); err != nil {
		// submission failed: revert inflight
		atomic.AddInt64(&v.inflight, -1)
		return err
	}
	return nil
}

// SubmitFunc convenience to submit a TaskFunc.
func (v *VirtualPool) SubmitFunc(f TaskFunc) error { return v.Submit(f) }

// CloseForSubmit marks this virtual pool as no longer accepting top-level submissions.
// Note: this does not close the shared pool; caller is responsible for closing the shared pool
// when all virtual pools are done (call SharedTaskPool.CloseForSubmit()).
func (v *VirtualPool) CloseForSubmit() {
	v.closed.Store(true)
	// if nothing inflight, close collector now
	if atomic.LoadInt64(&v.inflight) == 0 {
		v.collectorOnce.Do(func() {
			if v.ec != nil {
				v.ec.close()
			}
		})
	}
}

// isFailed returns true if this virtual pool encountered a fail-fast error.
func (v *VirtualPool) isFailed() bool {
	if p := v.firstErr.Load(); p != nil && *p != nil {
		return true
	}
	return false
}

func (v *VirtualPool) recordFirstError(err error) {
	ep := new(error)
	*ep = err
	v.firstErr.CompareAndSwap(nil, ep) // set only first
}

// FirstError returns the first encountered error for fail-fast virtual pools, or nil.
func (v *VirtualPool) FirstError() error {
	if p := v.firstErr.Load(); p != nil && *p != nil {
		return *p
	}
	return nil
}

// Errors returns a snapshot of collected errors for tolerant virtual pools.
// For fail-fast virtual pools this returns nil.
func (v *VirtualPool) Errors() []error {
	if v.ec == nil {
		return nil
	}
	return v.ec.Errors()
}

// ErrorChan returns the live channel of errors for tolerant mode, or nil for fail-fast mode.
func (v *VirtualPool) ErrorChan() <-chan error {
	if v.ec == nil {
		return nil
	}
	return v.ec.Ch
}
