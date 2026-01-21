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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// incrTask is a simple Task that increments a counter and optionally spawns nested child tasks.
type incrTask struct {
	counter *int64
	nested  int
	fail    bool
}

func (t *incrTask) Run(ctx context.Context, submit func(Task) error) error {
	atomic.AddInt64(t.counter, 1)
	if t.fail {
		return &poolTestError{msg: "fail"}
	}
	for i := 0; i < t.nested; i++ {
		if err := submit(&incrTask{counter: t.counter}); err != nil {
			return err
		}
	}
	return nil
}

type poolTestError struct{ msg string }

func (e *poolTestError) Error() string { return e.msg }

func TestVirtualPools_TolerantAndFailFast(t *testing.T) {
	ctx := context.Background()
	sp := NewSharedTaskPool(ctx, 4)

	// create virtual pools
	vt := sp.NewVirtualPool(VirtualTolerant)
	vf := sp.NewVirtualPool(VirtualFailFast)

	// submit tasks: tolerant pool will collect errors, fail pool will stop after first error
	var cntT int64
	var cntF int64

	// tolerant: submit 3 tasks, one of them fails
	if err := vt.Submit(&incrTask{counter: &cntT, nested: 0}); err != nil {
		t.Fatal(err)
	}
	if err := vt.Submit(&incrTask{counter: &cntT, nested: 0, fail: true}); err != nil {
		t.Fatal(err)
	}
	if err := vt.Submit(&incrTask{counter: &cntT, nested: 1}); err != nil {
		t.Fatal(err)
	}

	// fail-fast: submit tasks where second fails
	if err := vf.Submit(&incrTask{counter: &cntF, nested: 0}); err != nil {
		t.Fatal(err)
	}
	if err := vf.Submit(&incrTask{counter: &cntF, nested: 0, fail: true}); err != nil {
		t.Fatal(err)
	}
	// this submission after the failing one should be accepted but skipped when running
	if err := vf.Submit(&incrTask{counter: &cntF, nested: 0}); err != nil {
		t.Fatal(err)
	}

	// close virtuals for submit
	vt.CloseForSubmit()
	vf.CloseForSubmit()

	// close shared pool for submit and wait
	sp.CloseForSubmit()

	if err := sp.Wait(); err != nil {
		t.Fatalf("shared wait error: %v", err)
	}

	// tolerant: expect 4 increments (one of the tasks had nested:1 which is now allowed even after CloseForSubmit)
	if got := atomic.LoadInt64(&cntT); got != 4 {
		t.Fatalf("tolerant counter expected 4 got %d", got)
	}

	// tolerant: expect at least one error recorded
	if len(vt.Errors()) == 0 {
		t.Fatalf("expected tolerant errors, got none")
	}

	// fail-fast: at least one task should have executed and the virtual should have recorded a first error
	if got := atomic.LoadInt64(&cntF); got < 1 || got > 3 {
		t.Fatalf("fail-fast counter expected between 1 and 3 got %d", got)
	}
	if vf.FirstError() == nil {
		t.Fatalf("expected fail-fast virtual to record a first error")
	}
}

// Test that Wait blocks until the virtual pool is drained (tolerant mode).
func TestVirtualPool_Wait_Tolerant(t *testing.T) {
	ctx := context.Background()
	sp := NewSharedTaskPool(ctx, 2)
	vt := sp.NewVirtualPool(VirtualTolerant)

	var cnt int64
	// submit a few tasks
	if err := vt.Submit(&incrTask{counter: &cnt}); err != nil {
		t.Fatal(err)
	}
	if err := vt.Submit(&incrTask{counter: &cnt}); err != nil {
		t.Fatal(err)
	}

	// stop accepting submissions
	vt.CloseForSubmit()
	// Wait should block until inflight==0 and then return
	go func() {
		// close shared when we're done submitting all virtuals
		sp.CloseForSubmit()
	}()

	// Wait on virtual specifically
	vt.Wait()

	if got := atomic.LoadInt64(&cnt); got != 2 {
		t.Fatalf("expected 2 increments, got %d", got)
	}
}

// Test that Wait unblocks for a fail-fast virtual even if some queued tasks are skipped.
func TestVirtualPool_Wait_FailFast(t *testing.T) {
	ctx := context.Background()
	sp := NewSharedTaskPool(ctx, 2)
	vf := sp.NewVirtualPool(VirtualFailFast)

	var cnt int64
	// submit a task that will fail and another that would be skipped when run
	if err := vf.Submit(&incrTask{counter: &cnt}); err != nil {
		t.Fatal(err)
	}
	if err := vf.Submit(&incrTask{counter: &cnt, fail: true}); err != nil {
		t.Fatal(err)
	}
	if err := vf.Submit(&incrTask{counter: &cnt}); err != nil {
		t.Fatal(err)
	}

	vf.CloseForSubmit()
	sp.CloseForSubmit()

	// Wait should return once virtual is drained (some tasks may be skipped but inflight reaches 0)
	vf.Wait()

	if vf.FirstError() == nil {
		t.Fatalf("expected first error on fail-fast virtual")
	}
	if got := atomic.LoadInt64(&cnt); got < 1 {
		t.Fatalf("expected at least one task executed, got %d", got)
	}
}

func TestVirtualPool_CancellationHang(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sp := NewSharedTaskPool(ctx, 1)
	vp := sp.NewVirtualPool(VirtualFailFast)

	var wg sync.WaitGroup
	wg.Add(2)

	// Task 1: blocks until cancelled
	vp.SubmitFunc(func(ctx context.Context, submit func(Task) error) error {
		defer wg.Done()
		<-ctx.Done()
		return nil
	})

	// Task 2: should run even if cancelled (to decrement inflight)
	vp.SubmitFunc(func(ctx context.Context, submit func(Task) error) error {
		defer wg.Done()
		return nil
	})

	// Give time for Task 1 to start and Task 2 to be queued
	time.Sleep(100 * time.Millisecond)

	// Cancel the pool
	cancel()

	vp.CloseForSubmit()

	done := make(chan struct{})
	go func() {
		vp.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Wait returned successfully")
	case <-time.After(1 * time.Second):
		t.Fatal("Wait timed out - likely hung due to dropped task or negative inflight")
	}
}
