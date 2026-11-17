package types

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/tree/consts"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type Transaction struct {
	transactionId      string // ID that identifies the Transaction
	timerMutex         sync.Mutex
	timeout            time.Duration                 // delay until auto rollback, set via SetTimeout
	timerCtx           context.Context               // context the auto rollback runs with
	armed              bool                          // set once the transaction is committed and awaits Confirm
	timer              *time.Timer                   // auto rollback timer, nil until armed
	transactionManager *TransactionManager           // referernce to the TransactionManager this transaction is registered with
	newIntents         map[string]*TransactionIntent // new Intents with their content
	oldIntents         map[string]*TransactionIntent // old intents content
	oldRunning         *TransactionIntent            // old running config
	replace            *TransactionIntent            // possible replace config
	isRollback         bool                          // indicates if the transaction is already a rollback transaction.
	// Used to deactivate auto rollback triggering
}

func NewTransaction(id string, tm *TransactionManager) *Transaction {
	return &Transaction{
		transactionId:      id,
		transactionManager: tm,
		newIntents:         map[string]*TransactionIntent{},
		oldIntents:         map[string]*TransactionIntent{},
		oldRunning:         NewTransactionIntent(consts.RunningIntentName, 600),
		replace:            NewTransactionIntent(consts.ReplaceIntentName, consts.ReplaceValuesPrio),
	}
}

func (t *Transaction) GetNewIntents() map[string]*TransactionIntent {
	return t.newIntents
}

func (t *Transaction) GetOldRunning() *TransactionIntent {
	return t.oldRunning
}

func (t *Transaction) GetReplace() *TransactionIntent {
	return t.replace
}

func (t *Transaction) SetReplace(ti *TransactionIntent) {
	t.replace = ti
}

func (t *Transaction) IsRollback() bool {
	return t.isRollback
}

// Confirm disarms the auto rollback. It fails for a transaction that is not committed yet.
// Callers hold the TransactionManager lock.
func (t *Transaction) Confirm() error {
	t.timerMutex.Lock()
	defer t.timerMutex.Unlock()
	if !t.armed {
		return fmt.Errorf("transaction %s is not committed yet", t.transactionId)
	}
	t.stopTimerLocked()
	return nil
}

// stopTimer prevents a pending auto rollback from being triggered.
func (t *Transaction) stopTimer() {
	t.timerMutex.Lock()
	defer t.timerMutex.Unlock()
	t.stopTimerLocked()
}

func (t *Transaction) stopTimerLocked() {
	if t.timer != nil {
		t.timer.Stop()
	}
}

// StartRollbackTimer marks the transaction as committed and schedules the auto rollback
// after the timeout configured via SetTimeout. It can only be called once.
// The scheduled rollback re-validates under the TransactionManager lock that the
// transaction is still the registered one, so a Confirm or Cancel that stopped the
// timer too late still wins.
func (t *Transaction) StartRollbackTimer(ctx context.Context) error {
	t.timerMutex.Lock()
	defer t.timerMutex.Unlock()
	if t.armed {
		return fmt.Errorf("rollback timer of transaction %s already started", t.transactionId)
	}
	t.armed = true
	if t.timerCtx == nil {
		return nil
	}
	logf.FromContext(ctx).Info("transaction rollback timer started", "delay", t.timeout.String())
	t.timer = time.AfterFunc(t.timeout, func() {
		t.transactionManager.autoRollback(t.timerCtx, t)
	})
	return nil
}

// SetTimeout configures the delay after which an unconfirmed transaction is rolled back.
// Only the values of ctx (e.g. the logger) are kept. Its cancellation is dropped, because
// the request that created the transaction ends long before the timer fires.
func (t *Transaction) SetTimeout(ctx context.Context, d time.Duration) {
	t.timerMutex.Lock()
	defer t.timerMutex.Unlock()
	t.timeout = d
	t.timerCtx = context.WithoutCancel(ctx)
}

func (t *Transaction) GetIntentNames() []string {
	result := []string{}
	for k := range t.newIntents {
		result = append(result, k)
	}
	return result
}

func (t *Transaction) IntentCount() int {
	return len(t.newIntents)
}

// GetRollbackTransaction builds the transaction that reverts this one. It has no side effects on the timer.
func (t *Transaction) GetRollbackTransaction() *Transaction {
	tr := NewTransaction(t.GetTransactionId()+" - Rollback", t.transactionManager)
	for _, v := range t.oldIntents {
		_ = tr.AddTransactionIntent(v, TransactionIntentNew)
	}
	tr.isRollback = true
	return tr
}

func (t *Transaction) GetTransactionId() string {
	return t.transactionId
}

func (t *Transaction) IsNoOp() bool {
	return (t.IntentCount() == 0) && t.GetReplace() == nil
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
func (t *Transaction) AddIntentContent(name string, tit TransactionIntentType, priority int32, content []*treetypes.PathAndUpdate, explicitDeletes *sdcpb.PathSet) error {
	dstMap := t.getTransactionIntentTypeMap(tit)
	_, exists := dstMap[name]
	if exists {
		return fmt.Errorf("intent %s already exists in transaction", name)
	}
	ti := NewTransactionIntent(name, priority)
	dstMap[name] = ti

	ti.AddUpdates(content)
	ti.AddExplicitDeletes(explicitDeletes.ToPathSlice())
	return nil
}

func (t *Transaction) GetPathSet(tit TransactionIntentType) *sdcpb.PathSet {
	srcMap := t.getTransactionIntentTypeMap(tit)
	ps := &sdcpb.PathSet{}
	for _, intent := range srcMap {
		ps.Join(intent.GetPathSet())
	}
	return ps
}
