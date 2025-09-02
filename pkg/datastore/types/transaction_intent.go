package types

import (
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TransactionIntent struct {
	name string
	// updates is nil if the intent did not exist.
	updates treetypes.UpdateSlice
	delete  bool
	// onlyIntended, the orphan flag, delte only from intended store, but keep in device
	onlyIntended bool
	priority     int32
	// deviation indicates that the intent is a tolerated deviation.
	// it will be stored and used for change calculation but will be excluded when claculating actual deviations.
	deviation               bool
	deleteIgnoreNonExisting bool
	explicitDeletes         *sdcpb.PathSet
}

func NewTransactionIntent(name string, priority int32) *TransactionIntent {
	return &TransactionIntent{
		name:            name,
		updates:         make(treetypes.UpdateSlice, 0),
		priority:        priority,
		explicitDeletes: sdcpb.NewPathSet(),
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

func (ti *TransactionIntent) GetDeleteFlag() bool {
	return ti.delete
}

func (ti *TransactionIntent) GetDeletes() *sdcpb.PathSet {
	return ti.explicitDeletes
}

func (ti *TransactionIntent) GetOnlyIntended() bool {
	return ti.onlyIntended
}

func (ti *TransactionIntent) SetDeviation() {
	ti.deviation = true
}

func (ti *TransactionIntent) Deviation() bool {
	return ti.deviation
}

func (ti *TransactionIntent) SetDeleteIgnoreNonExisting() {
	ti.deleteIgnoreNonExisting = true
}

func (ti *TransactionIntent) GetDeleteIgnoreNonExisting() bool {
	return ti.deleteIgnoreNonExisting
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

func (ti *TransactionIntent) AddExplicitDeletes(p []*sdcpb.Path) {
	for _, x := range p {
		ti.explicitDeletes.AddPath(x)
	}
}
