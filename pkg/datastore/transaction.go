package datastore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

var (
	ErrTransactionOngoing error = errors.New("transaction ongoing")
)

type TransactionManager struct {
	tmMutex     *sync.Mutex
	transaction *Transaction
	rollbacker  RollbackInterface
}

func NewTransactionManager(r RollbackInterface) *TransactionManager {
	return &TransactionManager{
		tmMutex:    &sync.Mutex{},
		rollbacker: r,
	}
}

func (t *TransactionManager) RegisterTransaction(ctx context.Context, trans *Transaction) error {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	if t.transactionOngoing() {
		return ErrTransactionOngoing
	}

	// no Unlock, the created transaction locks the Mutex, which must explicitly be unlocked via FinishTransaction
	t.transaction = trans
	return nil
}

// transactionIsOngoing requires caller to acquire lock before calling
func (t *TransactionManager) transactionOngoing() bool {
	return t.transaction != nil
}

// cleanupTransaction the expectation is, that the caller is holding the lock for the TransactionManager
func (t *TransactionManager) cleanupTransaction(id string) error {
	// Perform checks by calling GetTransaction
	_, err := t.GetTransaction(id)
	if err != nil {
		return err
	}
	t.transaction = nil
	return nil
}

func (t *TransactionManager) Confirm(id string) error {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	t.transaction.Confirm()
	return t.cleanupTransaction(id)
}

func (t *TransactionManager) Cancel(ctx context.Context, id string) error {
	t.tmMutex.Lock()
	defer t.tmMutex.Unlock()
	rollbacktransAction := t.transaction.GetRollbackTransaction()

	_, err := t.rollbacker.transactionSet(ctx, rollbacktransAction, false)
	if err != nil {
		return err
	}
	return t.cleanupTransaction(id)
}

func (t *TransactionManager) GetTransaction(id string) (*Transaction, error) {
	if t.transaction == nil {
		return nil, fmt.Errorf("no active transaction")
	}
	if t.transaction.transactionId != id {
		return nil, fmt.Errorf("transaction id %s is invalid", id)
	}
	return t.transaction, nil
}

type TransactionIntentType int

const (
	TransactionIntentNew TransactionIntentType = iota // Starts at 0
	TransactionIntentOld                              // Incremented to 1
)

type Transaction struct {
	transactionId string
	timer         *TransactionCancelTimer
	newIntents    map[string]*TransactionIntent
	oldIntents    map[string]*TransactionIntent
	oldRunning    *TransactionIntent
	replace       *TransactionIntent
}

func NewTransaction(id string) *Transaction {
	return &Transaction{
		transactionId: id,
		newIntents:    map[string]*TransactionIntent{},
		oldIntents:    map[string]*TransactionIntent{},
		oldRunning:    NewTransactionIntent("oldrunning", 600),
		replace:       NewTransactionIntent(tree.ReplaceIntentName, tree.ReplaceValuesPrio),
	}
}

func (t *Transaction) Confirm() {
	t.timer.Stop()
}

func (t *Transaction) StartRollbackTimer() error {
	if t.timer != nil {
		return t.timer.Start()
	}
	return nil
}

func (t *Transaction) SetTimeout(d time.Duration, f func()) {
	t.timer = NewTransactionCancelTimer(d, f)
}

func (t *Transaction) GetIntentNames() []string {
	result := []string{}
	for k := range t.newIntents {
		result = append(result, k)
	}
	return result
}

func (t *Transaction) GetRollbackTransaction() *Transaction {
	t.timer.Stop()
	tr := NewTransaction(t.GetTransactionId() + " - Rollback")
	for _, v := range t.oldIntents {
		tr.AddTransactionIntent(v, TransactionIntentNew)
	}

	return tr
}

func (t *Transaction) GetTransactionId() string {
	return t.transactionId
}

func (t *Transaction) getTransactionIntentTypeMap(tit TransactionIntentType) map[string]*TransactionIntent {
	switch tit {
	case TransactionIntentNew:
		return t.newIntents
	case TransactionIntentOld:
		return t.oldIntents
	default:
		return nil
	}
}

func (t *Transaction) AddTransactionIntents(ti []*TransactionIntent, tit TransactionIntentType) error {
	for _, v := range ti {
		err := t.AddTransactionIntent(v, tit)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Transaction) AddTransactionIntent(ti *TransactionIntent, tit TransactionIntentType) error {
	dstMap := t.getTransactionIntentTypeMap(tit)
	_, exists := dstMap[ti.name]
	if exists {
		return fmt.Errorf("intent %s already exists in transaction", ti.name)
	}
	dstMap[ti.name] = ti
	return nil
}

// AddIntentContent add the content of an intent. If the intent did not exist, add the name of the intent and content == nil.
func (t *Transaction) AddIntentContent(name string, tit TransactionIntentType, priority int32, content tree.UpdateSlice) error {
	dstMap := t.getTransactionIntentTypeMap(tit)
	_, exists := dstMap[name]
	if exists {
		return fmt.Errorf("intent %s already exists in transaction", name)
	}
	ti := NewTransactionIntent(name, priority)
	dstMap[name] = ti

	ti.AddUpdates(content)
	return nil
}

func (t *Transaction) GetPathSet(tit TransactionIntentType) *tree.PathSet {
	srcMap := t.getTransactionIntentTypeMap(tit)
	ps := tree.NewPathSet()
	for _, intent := range srcMap {
		ps.Join(intent.GetPathSet())
	}
	return ps
}

type TransactionIntent struct {
	name string
	// updates is nil if the intent did not exist.
	updates      tree.UpdateSlice
	delete       bool
	onlyIntended bool
	priority     int32
}

func NewTransactionIntent(name string, priority int32) *TransactionIntent {
	return &TransactionIntent{
		name: name,
	}
}

func (ti *TransactionIntent) SetDeleteFlag() {
	ti.delete = true
}
func (ti *TransactionIntent) SetDeleteOnlyIntendedFlag() {
	ti.delete = true
	ti.onlyIntended = true
}

func (ti *TransactionIntent) GetPathSet() *tree.PathSet {
	return ti.updates.ToPathSet()
}

func (ti *TransactionIntent) AddUpdate(u *cache.Update) {
	ti.updates = append(ti.updates, u)
}

func (ti *TransactionIntent) AddUpdates(u tree.UpdateSlice) {
	ti.updates = append(ti.updates, u...)
}

type RollbackInterface interface {
	// TransactionRollback(ctx context.Context, transaction *Transaction) error
	transactionSet(ctx context.Context, transaction *Transaction, dryRun bool) (*sdcpb.TransactionSetResponse, error)
}

type TransactionCancelTimer struct {
	delay     time.Duration
	done      chan struct{}
	doneMutex *sync.Mutex
	fnc       func()
}

func NewTransactionCancelTimer(delay time.Duration, f func()) *TransactionCancelTimer {
	return &TransactionCancelTimer{
		delay:     delay,
		fnc:       f,
		doneMutex: &sync.Mutex{},
	}
}

func (t *TransactionCancelTimer) Start() error {
	t.doneMutex.Lock()
	defer t.doneMutex.Unlock()
	if t.done != nil {
		return fmt.Errorf("TransactionCancelTimer already started")
	}
	t.done = make(chan struct{})

	go func() {
		timer := time.NewTimer(t.delay)
		log.Debugf("TransactionCancelTimer started (%s)", t.delay.String())
		defer timer.Stop() // Ensure the timer is cleaned up

		select {
		case <-timer.C:
			// Timer fired, process TransactionCancel action
			log.Infof("TransactionCancelTimer triggered")
			if t.fnc != nil {
				t.fnc()
			}
		case <-t.done:
			// Stop the timer
			log.Debugf("TransactionCancelTimer stopped")
		}
	}()

	return nil
}

func (t *TransactionCancelTimer) Stop() {
	t.doneMutex.Lock()
	defer t.doneMutex.Unlock()

	if t.done == nil {
		return
	}
	close(t.done)
	t.done = nil
}
