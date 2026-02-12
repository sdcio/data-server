package types

import (
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TransactionIntent struct {
	name string
	// updates is nil if the intent did not exist.
	updates []*treetypes.PathAndUpdate
	delete  bool
	// onlyIntended, the orphan flag, delte only from intended store, but keep in device
	onlyIntended            bool
	priority                int32
	nonRevertive            bool
	deleteIgnoreNonExisting bool
	explicitDeletes         *sdcpb.PathSet
	previouslyApplied       bool
}

func NewTransactionIntent(name string, priority int32) *TransactionIntent {
	return &TransactionIntent{
		name:            name,
		updates:         make([]*treetypes.PathAndUpdate, 0),
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

func (ti *TransactionIntent) AddUpdates(u []*treetypes.PathAndUpdate) {
	ti.updates = append(ti.updates, u...)
}

func (ti *TransactionIntent) GetUpdates() []*treetypes.PathAndUpdate {
	return ti.updates
}

func (ti *TransactionIntent) GetDeleteFlag() bool {
	return ti.delete
}

func (ti *TransactionIntent) GetDeletes() *sdcpb.PathSet {
	return ti.explicitDeletes
}

func (ti *TransactionIntent) GetPreviouslyApplied() bool {
	return ti.previouslyApplied
}

func (ti *TransactionIntent) GetOnlyIntended() bool {
	return ti.onlyIntended
}

func (ti *TransactionIntent) SetNonRevertive() {
	ti.nonRevertive = true
}

func (ti *TransactionIntent) NonRevertive() bool {
	return ti.nonRevertive
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

func (ti *TransactionIntent) SetPreviouslyApplied() {
	ti.previouslyApplied = true
}

func (ti *TransactionIntent) SetDeleteOnlyIntendedFlag() {
	ti.delete = true
	ti.onlyIntended = true
}

func (ti *TransactionIntent) GetPathSet() *sdcpb.PathSet {
	result := sdcpb.NewPathSet()
	for _, upd := range ti.updates {
		result.AddPath(upd.GetPath())
	}
	return result
}

func (ti *TransactionIntent) AddUpdate(u *treetypes.PathAndUpdate) {
	ti.updates = append(ti.updates, u)
}

func (ti *TransactionIntent) AddExplicitDeletes(p []*sdcpb.Path) {
	for _, x := range p {
		ti.explicitDeletes.AddPath(x)
	}
}
