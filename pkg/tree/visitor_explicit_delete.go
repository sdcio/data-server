package tree

import (
	"context"
	"sync"

	"github.com/sdcio/data-server/pkg/utils"
)

type ExplicitDeleteVisitor struct {
	BaseVisitor
	owner    string
	priority int32

	// created entries for further stat calculation
	relatedLeafVariants LeafVariantSlice
	rlvMutex            *sync.Mutex
}

var _ EntryVisitor = (*ExplicitDeleteVisitor)(nil)

func NewExplicitDeleteVisitor(owner string, priority int32) *ExplicitDeleteVisitor {
	return &ExplicitDeleteVisitor{
		priority:            priority,
		owner:               owner,
		relatedLeafVariants: []*LeafEntry{},
		rlvMutex:            &sync.Mutex{},
	}
}

func (edv *ExplicitDeleteVisitor) Visit(ctx context.Context, e Entry) error {
	if !e.HoldsLeafvariants() {
		return nil
	}
	le := e.GetLeafVariantEntries().GetByOwner(edv.owner)
	if le != nil {
		le.MarkExpliciteDelete()
	} else {
		le = e.GetLeafVariantEntries().AddExplicitDeleteEntry(edv.owner, edv.priority)
	}
	edv.rlvMutex.Lock()
	edv.relatedLeafVariants = append(edv.relatedLeafVariants, le)
	edv.rlvMutex.Unlock()
	return nil
}

// GetExplicitDeleteCreationCount returns the amount of all the explicitDelete LeafVariants that where created.
func (edv *ExplicitDeleteVisitor) GetExplicitDeleteCreationCount() int {
	return len(edv.relatedLeafVariants)
}

// GetCreatedExplicitDeleteLeafEntries returns all the explicitDelete LeafVariants that where created.
func (edv *ExplicitDeleteVisitor) GetCreatedExplicitDeleteLeafEntries() LeafVariantSlice {
	return edv.relatedLeafVariants
}

// ExplicitDeleteVisitors map of *ExplicitDeleteVisitor indexed by the intent name
type ExplicitDeleteVisitors map[string]*ExplicitDeleteVisitor

func (e ExplicitDeleteVisitors) Stats() map[string]int {
	return utils.MapApplyFuncToMap(e, func(k string, v *ExplicitDeleteVisitor) int {
		return v.GetExplicitDeleteCreationCount()
	})
}

// type ExplicitDeleteTask struct {
// 	e        Entry
// 	owner    string
// 	priority int32
// }

// func NewExplicitDeleteTask(e Entry, owner string, priority int32) *ExplicitDeleteTask {
// 	return &ExplicitDeleteTask{
// 		e:        e,
// 		owner:    owner,
// 		priority: priority,
// 	}
// }

// func (edt *ExplicitDeleteTask) Run(ctx context.Context, submit func(pool.Task) error) error {

// 	if !edt.e.HoldsLeafvariants() {
// 		return nil
// 	}
// 	le := edt.e.GetLeafVariantEntries().GetByOwner(edt.owner)
// 	if le != nil {
// 		le.MarkExpliciteDelete()
// 	} else {
// 		le = edt.e.GetLeafVariantEntries().AddExplicitDeleteEntry(edt.owner, edt.priority)
// 	}

// 	for _, c := range edt.e.GetChilds(DescendMethodAll) {
// 		deleteTask := NewExplicitDeleteTask(c, edt.owner, edt.priority)
// 		err := submit(deleteTask)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil

// }

// func ExplicitDelete(ctx context.Context, e Entry, dps *DeletePathSet, pool pool.VirtualPoolI) error {
// 	for dp := range dps.Items() {
// 		for pi := range dp.PathItems() {
// 			edp, err := e.NavigateSdcpbPath(ctx, pi)
// 			if err != nil {
// 				return err
// 			}

// 			deleteTask := NewExplicitDeleteTask(edp, dp.GetOwner(), dp.GetPrio())

// 			err = pool.Submit(deleteTask)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	// close pool for additional external submission
// 	pool.CloseForSubmit()
// 	// wait for the pool to run dry
// 	pool.Wait()
// 	return nil
// }
