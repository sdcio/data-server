package datastore

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTryLockWithRetry_SucceedsAfterBriefContention(t *testing.T) {
	t.Parallel()

	var m sync.Mutex
	m.Lock()

	go func() {
		time.Sleep(40 * time.Millisecond)
		m.Unlock()
	}()

	locked, err := tryLockWithRetry(context.Background(), &m, 10, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("tryLockWithRetry() unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("tryLockWithRetry() = unlocked, want locked")
	}
	m.Unlock()
}

func TestTryLockWithRetry_FailsAfterRetries(t *testing.T) {
	t.Parallel()

	var m sync.Mutex
	m.Lock()
	defer m.Unlock()

	locked, err := tryLockWithRetry(context.Background(), &m, 3, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("tryLockWithRetry() unexpected error: %v", err)
	}
	if locked {
		t.Fatal("tryLockWithRetry() = locked, want unlocked")
	}
}

func TestTryLockWithRetry_ContextCanceled(t *testing.T) {
	t.Parallel()

	var m sync.Mutex
	m.Lock()
	defer m.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	locked, err := tryLockWithRetry(ctx, &m, 10, 10*time.Millisecond)
	if locked {
		t.Fatal("tryLockWithRetry() = locked, want unlocked")
	}
	if err == nil {
		t.Fatal("tryLockWithRetry() error = nil, want error")
	}
	if err != ErrContextDone {
		t.Fatalf("tryLockWithRetry() error = %v, want %v", err, ErrContextDone)
	}
}
