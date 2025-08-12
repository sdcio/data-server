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
	// deviation indicates that the intent is a tolerated deviation.
	// it will be stored and used for change calculation but will be excluded when claculating actual deviations.
	deviation               bool
	deleteIgnoreNonExisting bool
	deletes                 *sdcpb.PathSet
}

func NewTransactionIntent(name string, priority int32) *TransactionIntent {
	return &TransactionIntent{
		name:     name,
		updates:  make(treetypes.UpdateSlice, 0),
		priority: priority,
		deletes:  sdcpb.NewPathSet(),
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

func (ti *TransactionIntent) GetDeletes() *sdcpb.PathSet {
	return ti.deletes
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

func (ti *TransactionIntent) GetPathSet() *treetypes.PathSet {
	return ti.updates.ToPathSet()
}

func (ti *TransactionIntent) AddUpdate(u *treetypes.Update) {
	ti.updates = append(ti.updates, u)
}

func (ti *TransactionIntent) AddDeletes(p []*sdcpb.Path) {
	for _, x := range p {
		ti.deletes.AddPath(x)
	}
}
