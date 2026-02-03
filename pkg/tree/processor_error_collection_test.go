package tree

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/pool"
)

// TestProcessorErrorCollection verifies that processor error collection happens
// AFTER all tasks complete, not before. This ensures CloseAndWait is called
// before checking pool.Errors().
func TestProcessorErrorCollection(t *testing.T) {
	// Create a shared task pool
	ctx := context.Background()
	sharedPool := pool.NewSharedTaskPool(ctx, 2)

	// Test with tolerant mode to collect multiple errors
	vpool := sharedPool.NewVirtualPool(pool.VirtualTolerant)

	var errorCount atomic.Int32
	var taskCompletionCount atomic.Int32

	// Submit tasks that will error with a delay
	// This simulates tasks that take time to complete and add errors
	for i := 0; i < 5; i++ {
		err := vpool.Submit(pool.TaskFunc(func(ctx context.Context, submit func(pool.Task) error) error {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			taskCompletionCount.Add(1)
			errorCount.Add(1)
			return errors.New("task error")
		}))
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}
	}

	// Close and wait - this MUST complete before we check errors
	vpool.CloseAndWait()

	// Now verify all errors are collected
	errs := vpool.Errors()

	// All 5 tasks should have completed
	if count := taskCompletionCount.Load(); count != 5 {
		t.Errorf("Expected 5 tasks to complete, got %d", count)
	}

	// All 5 errors should be collected
	if len(errs) != 5 {
		t.Errorf("Expected 5 errors, got %d", len(errs))
	}

	// Verify combined error works
	combinedErr := errors.Join(errs...)
	if combinedErr == nil {
		t.Error("Expected combined error to be non-nil")
	}

	sharedPool.CloseForSubmit()
	sharedPool.Wait()
}

// TestProcessorEarlyReturnCleanup verifies that if Submit fails early,
// the pool is still properly cleaned up.
func TestProcessorEarlyReturnCleanup(t *testing.T) {
	ctx := context.Background()
	sharedPool := pool.NewSharedTaskPool(ctx, 2)

	vpool := sharedPool.NewVirtualPool(pool.VirtualFailFast)

	// Close the pool immediately so Submit will fail
	vpool.CloseForSubmit()

	// Try to submit - this should fail
	err := vpool.Submit(pool.TaskFunc(func(ctx context.Context, submit func(pool.Task) error) error {
		return nil
	}))

	if err == nil {
		t.Fatal("Expected Submit to fail on closed pool")
	}

	// Even though Submit failed, we should be able to call CloseAndWait safely
	vpool.CloseAndWait()

	// Wait should not block
	done := make(chan struct{})
	go func() {
		vpool.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - Wait unblocked
	case <-time.After(1 * time.Second):
		t.Fatal("Wait blocked after CloseAndWait on failed Submit")
	}

	sharedPool.CloseForSubmit()
	sharedPool.Wait()
}
