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
	vt := sp.NewVirtualPool("tolerant", VirtualTolerant, 16)
	vf := sp.NewVirtualPool("fail", VirtualFailFast, 0)

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

	// drain tolerant error channel live
	var seenErrs int32
	done := make(chan struct{})
	go func() {
		for e := range vt.ErrorChan() {
			if e != nil {
				atomic.AddInt32(&seenErrs, 1)
			}
		}
		close(done)
	}()

	if err := sp.Wait(); err != nil {
		t.Fatalf("shared wait error: %v", err)
	}

	// tolerant collector channel is closed automatically when virtual is closed and inflight reaches zero
	// wait for drain goroutine
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for tolerant drain")
	}

	// tolerant: expect 3 increments (one of them had nested:1 so total 3)
	if got := atomic.LoadInt64(&cntT); got != 3 {
		t.Fatalf("tolerant counter expected 3 got %d", got)
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
