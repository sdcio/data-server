package types

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const testTimerDelay = 20 * time.Millisecond

type fakeRollbacker struct {
	calls  atomic.Int32
	err    error
	ctxErr chan error
}

func (f *fakeRollbacker) TransactionRollback(ctx context.Context, _ *Transaction, _ bool) (*sdcpb.TransactionSetResponse, error) {
	f.calls.Add(1)
	if f.ctxErr != nil {
		f.ctxErr <- ctx.Err()
	}
	return nil, f.err
}

// newCommittedTransaction registers a transaction with an intent and starts its rollback timer.
func newCommittedTransaction(t *testing.T, tm *TransactionManager, id string, delay time.Duration) *Transaction {
	t.Helper()
	ctx := context.Background()
	tr := NewTransaction(id, tm)
	tr.SetReplace(nil)
	if err := tr.AddTransactionIntent(NewTransactionIntent("intent", 10), TransactionIntentNew); err != nil {
		t.Fatal(err)
	}
	tr.SetTimeout(ctx, delay)
	if _, err := tm.RegisterTransaction(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if err := tr.StartRollbackTimer(ctx); err != nil {
		t.Fatal(err)
	}
	return tr
}

func registered(tm *TransactionManager) *Transaction {
	tm.tmMutex.Lock()
	defer tm.tmMutex.Unlock()
	return tm.transaction
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(time.Millisecond)
	}
}

// The auto rollback must not inherit the cancellation of the request that created the transaction.
func TestAutoRollbackSurvivesRequestContextCancel(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	rb := &fakeRollbacker{ctxErr: make(chan error, 1)}
	tm := NewTransactionManager(rb)
	tr := NewTransaction("A", tm)
	tr.SetReplace(nil)
	_ = tr.AddTransactionIntent(NewTransactionIntent("intent", 10), TransactionIntentNew)
	tr.SetTimeout(reqCtx, testTimerDelay)
	if _, err := tm.RegisterTransaction(reqCtx, tr); err != nil {
		t.Fatal(err)
	}
	if err := tr.StartRollbackTimer(reqCtx); err != nil {
		t.Fatal(err)
	}
	cancel()

	select {
	case err := <-rb.ctxErr:
		if err != nil {
			t.Errorf("auto rollback ran with dead context: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rollback never ran")
	}
	waitFor(t, func() bool { return registered(tm) == nil })
}

// A Confirm that lands after the timer fired, but before the blocked auto rollback got
// its locks, must prevent the rollback and not panic.
func TestConfirmRacingTimerFire(t *testing.T) {
	rb := &fakeRollbacker{}
	lock := &sync.Mutex{}
	tm := NewTransactionManager(rb, WithAutoRollbackLock(lock))

	lock.Lock() // parks the fired timer callback
	newCommittedTransaction(t, tm, "A", time.Millisecond)
	time.Sleep(10 * testTimerDelay)

	if err := tm.Confirm("A"); err != nil {
		t.Fatal(err)
	}
	lock.Unlock()

	time.Sleep(testTimerDelay)
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("confirmed transaction was rolled back (calls=%d)", n)
	}
}

func TestConfirmWrongIDKeepsAutoRollbackArmed(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	newCommittedTransaction(t, tm, "A", testTimerDelay)

	if err := tm.Confirm("B"); err == nil {
		t.Fatal("expected error for wrong id")
	}

	waitFor(t, func() bool { return rb.calls.Load() == 1 })
	waitFor(t, func() bool { return registered(tm) == nil })
}

func TestConfirmBeforeCommitFails(t *testing.T) {
	tm := NewTransactionManager(&fakeRollbacker{})
	tr := NewTransaction("A", tm)
	if _, err := tm.RegisterTransaction(context.Background(), tr); err != nil {
		t.Fatal(err)
	}

	if err := tm.Confirm("A"); err == nil {
		t.Fatal("expected error confirming an uncommitted transaction")
	}
	if registered(tm) != tr {
		t.Error("uncommitted transaction was unregistered by Confirm")
	}
}

func TestConfirmUnregistersAndStopsRollback(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	newCommittedTransaction(t, tm, "A", testTimerDelay)

	if err := tm.Confirm("A"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * testTimerDelay)
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("confirmed transaction was rolled back (calls=%d)", n)
	}
	if registered(tm) != nil {
		t.Error("confirmed transaction still registered")
	}
}

func TestCancelWrongIDHasNoSideEffects(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	tr := newCommittedTransaction(t, tm, "A", time.Minute)

	if err := tm.Cancel(context.Background(), "B"); err == nil {
		t.Fatal("expected error for wrong id")
	}
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("Cancel with wrong id rolled back the active transaction (calls=%d)", n)
	}
	if registered(tm) != tr {
		t.Error("active transaction was unregistered")
	}
}

func TestCancelRollsBackAndStopsTimer(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	newCommittedTransaction(t, tm, "A", testTimerDelay)

	if err := tm.Cancel(context.Background(), "A"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * testTimerDelay)
	if n := rb.calls.Load(); n != 1 {
		t.Errorf("expected exactly one rollback, got %d", n)
	}
	if registered(tm) != nil {
		t.Error("cancelled transaction still registered")
	}
}

// A failed Cancel keeps the transaction registered with its timer armed, so nothing gets stuck.
func TestCancelFailureKeepsTransactionAndTimer(t *testing.T) {
	rb := &fakeRollbacker{err: errors.New("device down")}
	tm := NewTransactionManager(rb)
	tr := newCommittedTransaction(t, tm, "A", 50*testTimerDelay)

	if err := tm.Cancel(context.Background(), "A"); err == nil {
		t.Fatal("expected rollback error")
	}
	if registered(tm) != tr {
		t.Fatal("transaction was unregistered despite failed rollback")
	}

	rb.err = nil
	if err := tm.Cancel(context.Background(), "A"); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if registered(tm) != nil {
		t.Error("transaction still registered after successful retry")
	}
}

func TestAutoRollbackFailureKeepsTransactionRegistered(t *testing.T) {
	rb := &fakeRollbacker{err: errors.New("device down")}
	tm := NewTransactionManager(rb)
	tr := newCommittedTransaction(t, tm, "A", testTimerDelay)

	waitFor(t, func() bool { return rb.calls.Load() == 1 })
	time.Sleep(testTimerDelay)
	if registered(tm) != tr {
		t.Error("transaction was dropped although the rollback failed")
	}
}

// A late timer of an old transaction must not unregister the transaction that replaced it.
func TestAutoRollbackIgnoresStaleTransaction(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	old := NewTransaction("A", tm)
	b := newCommittedTransaction(t, tm, "B", time.Minute)

	tm.autoRollback(context.Background(), old)

	if registered(tm) != b {
		t.Error("auto rollback of A unregistered unrelated transaction B")
	}
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("stale auto rollback reached the device (calls=%d)", n)
	}
}

func TestAutoRollbackWaitsForAutoRollbackLock(t *testing.T) {
	rb := &fakeRollbacker{}
	lock := &sync.Mutex{}
	tm := NewTransactionManager(rb, WithAutoRollbackLock(lock))

	lock.Lock()
	newCommittedTransaction(t, tm, "A", time.Millisecond)
	time.Sleep(10 * testTimerDelay)
	if n := rb.calls.Load(); n != 0 {
		t.Fatalf("auto rollback ran without holding the set-path lock (calls=%d)", n)
	}
	// RegisterTransaction must stay non-blocking while the rollback waits for the lock
	if _, err := tm.RegisterTransaction(context.Background(), NewTransaction("B", tm)); !errors.Is(err, ErrTransactionOngoing) {
		t.Errorf("expected ErrTransactionOngoing, got %v", err)
	}
	lock.Unlock()

	waitFor(t, func() bool { return rb.calls.Load() == 1 })
	waitFor(t, func() bool { return registered(tm) == nil })
}

// A transaction that changed nothing is only unregistered on timeout; the device is not contacted.
func TestAutoRollbackNoOpTransaction(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	ctx := context.Background()
	tr := NewTransaction("A", tm)
	tr.SetReplace(nil)
	tr.SetTimeout(ctx, testTimerDelay)
	if _, err := tm.RegisterTransaction(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if err := tr.StartRollbackTimer(ctx); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return registered(tm) == nil })
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("no-op transaction reached the device (calls=%d)", n)
	}
}

func TestGuardCleanupStopsTimer(t *testing.T) {
	rb := &fakeRollbacker{}
	tm := NewTransactionManager(rb)
	ctx := context.Background()
	tr := NewTransaction("A", tm)
	tr.SetTimeout(ctx, testTimerDelay)
	guard, err := tm.RegisterTransaction(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.StartRollbackTimer(ctx); err != nil {
		t.Fatal(err)
	}

	guard.Done()

	time.Sleep(5 * testTimerDelay)
	if n := rb.calls.Load(); n != 0 {
		t.Errorf("rollback ran for an unregistered transaction (calls=%d)", n)
	}
	if registered(tm) != nil {
		t.Error("guard did not unregister the transaction")
	}
}
