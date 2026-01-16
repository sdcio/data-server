package types

import (
	"context"
	"fmt"
	"time"

	"github.com/sdcio/data-server/pkg/tree"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type Transaction struct {
	transactionId      string                        // ID that identifies the Transaction
	timer              *TransactionCancelTimer       // timer that triggers auto rollback
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
		oldRunning:         NewTransactionIntent(tree.RunningIntentName, 600),
		replace:            NewTransactionIntent(tree.ReplaceIntentName, tree.ReplaceValuesPrio),
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

func (t *Transaction) Confirm() error {
	if t.timer == nil {
		return fmt.Errorf("no ongoing transaction")
	}
	t.timer.Stop()
	return nil
}

func (t *Transaction) rollback(ctx context.Context) func() {
	return func() {
		log := logger.FromContext(ctx)
		err := t.transactionManager.Rollback(ctx, t.GetRollbackTransaction(ctx))
		log.Error(err, "rollback failed")
	}
}

func (t *Transaction) StartRollbackTimer(ctx context.Context) error {
	if t.timer != nil {
		return t.timer.Start(ctx)
	}
	return nil
}

func (t *Transaction) SetTimeout(ctx context.Context, d time.Duration) {
	t.timer = NewTransactionCancelTimer(d, t.rollback(ctx))
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

func (t *Transaction) GetRollbackTransaction(ctx context.Context) *Transaction {
	t.timer.Stop()
	tr := NewTransaction(t.GetTransactionId()+" - Rollback", t.transactionManager)
	for _, v := range t.oldIntents {
		err := tr.AddTransactionIntent(v, TransactionIntentNew)
		if err != nil {
			log := logger.FromContext(ctx)
			log.Error(err, "failed getting rollback transaction", "intent", v.name)
		}
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
