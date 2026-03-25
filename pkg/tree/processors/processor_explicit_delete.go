package processors

import (
	"context"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
)

type ExplicitDeleteProcessor struct {
	params *explicitDeleteTaskContext
}

func NewExplicitDeleteProcessor(params *ExplicitDeleteTaskParams) *ExplicitDeleteProcessor {
	return &ExplicitDeleteProcessor{
		params: newExplicitDeleteTaskContext(params),
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

// ExplicitDeleteTaskParams contains the user-provided parameters for the explicit delete operation.
type ExplicitDeleteTaskParams struct {
	Owner    string
	Priority int32
}

// explicitDeleteTaskContext embeds the input parameters and adds operational/statistics data for the explicit delete process.
type explicitDeleteTaskContext struct {
	ExplicitDeleteTaskParams
	relatedLeafVariants api.LeafVariantSlice
	rlvMutex            sync.Mutex
}

func newExplicitDeleteTaskContext(p *ExplicitDeleteTaskParams) *explicitDeleteTaskContext {
	return &explicitDeleteTaskContext{
		ExplicitDeleteTaskParams: *p,
		relatedLeafVariants:      api.LeafVariantSlice{},
		rlvMutex:                 sync.Mutex{},
	}
}

func (p *ExplicitDeleteProcessor) Run(ctx context.Context, e api.Entry, poolFactory pool.VirtualPoolFactory) error {
	taskpool := poolFactory.NewVirtualPool(pool.VirtualTolerant)
	err := taskpool.Submit(newExplicitDeleteTask(e, p.params))
	taskpool.CloseAndWait()
	return err
}

type explicitDeleteTask struct {
	entry   api.Entry
	context *explicitDeleteTaskContext
}

func newExplicitDeleteTask(entry api.Entry, context *explicitDeleteTaskContext) *explicitDeleteTask {
	return &explicitDeleteTask{
		entry:   entry,
		context: context,
	}
}

func (t *explicitDeleteTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ops.HoldsLeafVariants(t.entry) {
		le := t.entry.GetLeafVariants().GetByOwner(t.context.Owner)
		if le != nil {
			le.MarkExpliciteDelete()
		} else {
			le = t.entry.GetLeafVariants().AddExplicitDeleteEntry(t.context.Owner, t.context.Priority)
		}
		t.context.rlvMutex.Lock()
		t.context.relatedLeafVariants = append(t.context.relatedLeafVariants, le)
		t.context.rlvMutex.Unlock()
	}

	// trigger the execution on all childs
	for _, c := range t.entry.GetChilds(types.DescendMethodAll) {
		err := submit(newExplicitDeleteTask(c, t.context))
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
