package types

import (
	"context"
	"errors"
	"fmt"
	"sync"

	logf "github.com/sdcio/logger"
)

var (
	ErrTransactionOngoing error = errors.New("transaction ongoing")
)

// TransactionManager tracks the single registered Transaction and arbitrates
// Confirm, Cancel and the timer-driven auto rollback, so that exactly one of them
// ends a transaction.
//
// Lock order: the set-path lock (see WithAutoRollbackLock) before tmMutex.
type TransactionManager struct {
	tmMutex     *sync.Mutex
	transaction *Transaction
	rollbacker  RollbackInterface
	// autoRollbackLock is taken (blocking) by the auto rollback before tmMutex.
	autoRollbackLock sync.Locker
}

// TransactionManagerOption configures optional behavior of a TransactionManager.
type TransactionManagerOption func(*TransactionManager)

// WithAutoRollbackLock makes the timer-driven rollback hold l while it sends the
// rollback to the device. l is the lock that serializes device writes; Cancel
// callers are expected to hold it already.
func WithAutoRollbackLock(l sync.Locker) TransactionManagerOption {
	return func(t *TransactionManager) {
		t.autoRollbackLock = l
	}
}

func NewTransactionManager(r RollbackInterface, opts ...TransactionManagerOption) *TransactionManager {
	t := &TransactionManager{
		tmMutex:    &sync.Mutex{},
		rollbacker: r,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// RegisterTransaction registers trans as the active transaction. It never blocks on
// a device round trip and fails with ErrTransactionOngoing if another transaction is
// registered. The returned guard unregisters the transaction unless Success() is called.
func (t *TransactionManager) RegisterTransaction(ctx context.Context, trans *Transaction) (*TransactionGuard, error) {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	log := logf.FromContext(ctx)
	if t.transactionOngoing() {
		return nil, ErrTransactionOngoing
	}

	t.transaction = trans

	return NewTransactionGuard(func() {
		log.Info("transaction canceling due to error")
		t.tmMutex.Lock()
		defer t.tmMutex.Unlock()
		trans.stopTimer()
		err := t.cleanupTransaction(trans.transactionId)
		if err != nil {
			log.Error(err, "error cleaning up transaction")
		}
	}), nil
}

// transactionOngoing requires caller to acquire lock before calling
func (t *TransactionManager) transactionOngoing() bool {
	return t.transaction != nil
}

// cleanupTransaction unregisters the transaction with the given id.
// The caller holds tmMutex.
func (t *TransactionManager) cleanupTransaction(id string) error {
	if _, err := t.getTransaction(id); err != nil {
		return err
	}
	t.transaction = nil
	return nil
}

// Confirm keeps the changes of the transaction and unregisters it. It has no side
// effect if id does not match the registered transaction.
func (t *TransactionManager) Confirm(id string) error {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	trans, err := t.getTransaction(id)
	if err != nil {
		return err
	}
	if err := trans.Confirm(); err != nil {
		return err
	}
	return t.cleanupTransaction(id)
}

// Cancel rolls the transaction back and unregisters it. The caller holds the lock that
// serializes device writes. If the rollback fails the transaction stays registered
// with its timer armed, so Cancel can be retried or the auto rollback tries again.
func (t *TransactionManager) Cancel(ctx context.Context, id string) error {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	trans, err := t.getTransaction(id)
	if err != nil {
		return err
	}

	if err := t.rollbackLocked(ctx, trans); err != nil {
		return err
	}
	trans.stopTimer()
	return t.cleanupTransaction(id)
}

// getTransaction returns the registered transaction if its id matches. The caller holds tmMutex.
func (t *TransactionManager) getTransaction(id string) (*Transaction, error) {
	if t.transaction == nil {
		return nil, fmt.Errorf("no active transaction")
	}
	if t.transaction.transactionId != id {
		return nil, fmt.Errorf("transaction id %s is invalid", id)
	}
	return t.transaction, nil
}

// rollbackLocked reverts trans on the device. Transactions that did not change anything are
// not sent to the device. The caller holds tmMutex.
func (t *TransactionManager) rollbackLocked(ctx context.Context, trans *Transaction) error {
	if trans.IsNoOp() {
		return nil
	}
	_, err := t.rollbacker.TransactionRollback(ctx, trans.GetRollbackTransaction(), false)
	return err
}

// autoRollback is invoked by the timer of trans. It does nothing if trans is no longer the
// registered transaction, i.e. it was confirmed, cancelled or unregistered while the timer
// was firing. If the rollback fails, trans stays registered so the failure stays visible
// and can be retried via Cancel.
func (t *TransactionManager) autoRollback(ctx context.Context, trans *Transaction) {
	log := logf.FromContext(ctx).WithValues("transaction-id", trans.transactionId)

	if t.autoRollbackLock != nil {
		t.autoRollbackLock.Lock()
		defer t.autoRollbackLock.Unlock()
	}
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()

	if t.transaction != trans {
		log.V(logf.VDebug).Info("transaction no longer active, skipping auto rollback")
		return
	}

	log.Info("transaction not confirmed in time, rolling back")
	if err := t.rollbackLocked(ctx, trans); err != nil {
		log.Error(err, "auto rollback failed, transaction stays registered")
		return
	}
	t.transaction = nil
}
