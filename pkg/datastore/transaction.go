package datastore

import (
	"fmt"
	"sync"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
)

type TransactionManager struct {
	transactionMutex sync.Mutex
	transaction      *Transaction
}

func NewTransactionManager() *TransactionManager {
	return &TransactionManager{}
}

func (t *TransactionManager) CreateTransaction(id string) *Transaction {
	t.transactionMutex.Lock()
	// no Unlock, the created transaction locks the Mutex, which must explicitly be unlocked via FinishTransaction
	t.transaction = NewTransaction(id)
	return t.transaction
}

func (t *TransactionManager) FinishTransaction(id string) error {
	// Perform checks by calling GetTransaction
	_, err := t.GetTransaction(id)
	if err != nil {
		return err
	}
	t.transaction = nil
	t.transactionMutex.Unlock()
	return nil
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

type Transaction struct {
	transactionId string
	intents       map[string]*TransactionIntent
}

func NewTransaction(id string) *Transaction {
	return &Transaction{
		transactionId: id,
	}
}

func (t *Transaction) GetIntentNames() []string {
	result := []string{}
	for k := range t.intents {
		result = append(result, k)
	}
	return result
}

func (t *Transaction) GetTransactionId() string {
	return t.transactionId
}

func (t *Transaction) AddTransactionIntent(ti *TransactionIntent) error {
	_, exists := t.intents[ti.name]
	if exists {
		return fmt.Errorf("intent %s already exists in transaction", ti.name)
	}
	t.intents[ti.name] = ti
	return nil
}

// AddIntentContent add the content of an intent. If the intent did not exist, add the name of the intent and content == nil.
func (t *Transaction) AddIntentContent(name string, priority int32, content tree.UpdateSlice) error {
	_, exists := t.intents[name]
	if exists {
		return fmt.Errorf("intent %s already exists in transaction", name)
	}
	ti := NewTransactionIntent(name, priority)
	t.intents[name] = ti

	ti.AddUpdates(content)
	return nil
}

func (t *Transaction) GetPathSet() *tree.PathSet {
	ps := tree.NewPathSet()
	for _, intent := range t.intents {
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
