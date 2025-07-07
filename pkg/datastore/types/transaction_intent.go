package types

import (
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
)

type TransactionIntent struct {
	name string
	// updates is nil if the intent did not exist.
	updates treetypes.UpdateSlice
	delete  bool
	// onlyIntended, the orphan flag, delte only from intended store, but keep in device
	onlyIntended bool
	priority     int32
	// doNotStore, mark the intent as a volatile, hence it is being used in the calculation in the tree,
	// but it is not stored in the cache. This is mainly use for deviation that are meant to continue to exist,
	// but should remain to be reported as deviations.
	doNotStore bool
}

func NewTransactionIntent(name string, priority int32) *TransactionIntent {
	return &TransactionIntent{
		name:     name,
		updates:  make(treetypes.UpdateSlice, 0),
		priority: priority,
	}
}

func (ti *TransactionIntent) GetName() string {
	return ti.name
}

func (ti *TransactionIntent) GetPriority() int32 {
	return ti.priority
}

func (ti *TransactionIntent) AddUpdates(u treetypes.UpdateSlice) {
	ti.updates = append(ti.updates, u...)
}

func (ti *TransactionIntent) GetUpdates() treetypes.UpdateSlice {
	return ti.updates
}

func (ti *TransactionIntent) GetOnlyIntended() bool {
	return ti.onlyIntended
}

func (ti *TransactionIntent) SetDoNotStoreFlag() {
	ti.doNotStore = true
}

func (ti *TransactionIntent) DoNotStore() bool {
	return ti.doNotStore
}

func (ti *TransactionIntent) SetDeleteFlag() {
	ti.delete = true
}
func (ti *TransactionIntent) SetDeleteOnlyIntendedFlag() {
	ti.delete = true
	ti.onlyIntended = true
}

func (ti *TransactionIntent) GetPathSet() *treetypes.PathSet {
	return ti.updates.ToPathSet()
}

func (ti *TransactionIntent) AddUpdate(u *treetypes.Update) {
	ti.updates = append(ti.updates, u)
}
