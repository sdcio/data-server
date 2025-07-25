package types

import (
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TransactionIntent struct {
	name string
	// updates is nil if the intent did not exist.
	updates      treetypes.UpdateSlice
	delete       bool
	onlyIntended bool
	priority     int32
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

func (ti *TransactionIntent) SetDeleteFlag() {
	ti.delete = true
}
func (ti *TransactionIntent) SetDeleteOnlyIntendedFlag() {
	ti.delete = true
	ti.onlyIntended = true
}

func (ti *TransactionIntent) GetPathSet() *sdcpb.PathSet {
	return ti.updates.ToSdcpbPathSet()
}

func (ti *TransactionIntent) AddUpdate(u *treetypes.Update) {
	ti.updates = append(ti.updates, u)
}
