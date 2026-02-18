package tree

import (
	"context"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
)

type ExplicitDeleteProcessor struct {
	params *ExplicitDeleteTaskParameters
}

func NewExplicitDeleteProcessor(params *ExplicitDeleteTaskParameters) *ExplicitDeleteProcessor {
	return &ExplicitDeleteProcessor{
		params: params,
	}
}

// GetExplicitDeleteCreationCount returns the amount of all the explicitDelete LeafVariants that where created.
func (edp *ExplicitDeleteProcessor) GetExplicitDeleteCreationCount() int {
	return len(edp.params.relatedLeafVariants)
}

// GetCreatedExplicitDeleteLeafEntries returns all the explicitDelete LeafVariants that where created.
func (edp *ExplicitDeleteProcessor) GetCreatedExplicitDeleteLeafEntries() api.LeafVariantSlice {
	return edp.params.relatedLeafVariants
}

type ExplicitDeleteTaskParameters struct {
	owner               string
	priority            int32
	relatedLeafVariants api.LeafVariantSlice
	rlvMutex            *sync.Mutex
}

func NewExplicitDeleteTaskParameters(owner string, priority int32) *ExplicitDeleteTaskParameters {
	return &ExplicitDeleteTaskParameters{
		priority:            priority,
		owner:               owner,
		relatedLeafVariants: api.LeafVariantSlice{},
		rlvMutex:            &sync.Mutex{},
	}
}

func (p *ExplicitDeleteProcessor) Run(ctx context.Context, e api.Entry, poolFactory pool.VirtualPoolFactory) error {
	taskpool := poolFactory.NewVirtualPool(pool.VirtualTolerant)
	err := taskpool.Submit(newExplicitDeleteTask(e, p.params))
	taskpool.CloseAndWait()
	return err
}

type explicitDeleteTask struct {
	entry  api.Entry
	params *ExplicitDeleteTaskParameters
}

func newExplicitDeleteTask(entry api.Entry, params *ExplicitDeleteTaskParameters) *explicitDeleteTask {
	return &explicitDeleteTask{
		entry:  entry,
		params: params,
	}
}

func (t *explicitDeleteTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if t.entry.HoldsLeafvariants() {
		le := t.entry.GetLeafVariants().GetByOwner(t.params.owner)
		if le != nil {
			le.MarkExpliciteDelete()
		} else {
			le = t.entry.GetLeafVariants().AddExplicitDeleteEntry(t.params.owner, t.params.priority)
		}
		t.params.rlvMutex.Lock()
		t.params.relatedLeafVariants = append(t.params.relatedLeafVariants, le)
		t.params.rlvMutex.Unlock()
	}

	// trigger the execution on all childs
	for _, c := range t.entry.GetChilds(types.DescendMethodAll) {
		err := submit(newExplicitDeleteTask(c, t.params))
		if err != nil {
			return err
		}
	}

	return nil
}

// Stats structs
type ExplicitDeleteProcessorStat interface {
	GetCreatedExplicitDeleteLeafEntries() api.LeafVariantSlice
	GetExplicitDeleteCreationCount() int
}

type ExplicitDeleteProcessorStatCollection map[string]ExplicitDeleteProcessorStat

func (e ExplicitDeleteProcessorStatCollection) Stats() map[string]int {
	return utils.MapApplyFuncToMap(e, func(k string, v ExplicitDeleteProcessorStat) int {
		return v.GetExplicitDeleteCreationCount()
	})
}

func (e ExplicitDeleteProcessorStatCollection) Count() int {
	count := 0
	for _, stat := range e {
		count += stat.GetExplicitDeleteCreationCount()
	}
	return count
}

func (e ExplicitDeleteProcessorStatCollection) ContainsEntries() bool {
	for _, stat := range e {
		if stat.GetExplicitDeleteCreationCount() > 0 {
			return true
		}
	}
	return false
}
