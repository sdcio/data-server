package processors

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// RemoveDeletedProcessorParams contains the user-provided parameters for the remove deleted operation.
type RemoveDeletedProcessorParams struct {
	Owner string
}

// RemoveDeletedProcessor is responsible for removing leaf variant entries marked for deletion by the specified owner.
type RemoveDeletedProcessor struct {
	context *removeDeletedTaskContext
}

func NewRemoveDeletedProcessor(params *RemoveDeletedProcessorParams) *RemoveDeletedProcessor {
	return &RemoveDeletedProcessor{
		context: newRemoveDeletedTaskContext(params),
	}
}

// GetDeleteStatsCount returns the amount of leaf variant entries that were removed during the RemoveDeleted process.
func (p *RemoveDeletedProcessor) GetDeleteStatsCount() int64 {
	return p.context.GetDeleteStatsCount()
}

// GetZeroLengthLeafVariantEntries returns the entries that have zero-length leaf variant entries after removal
func (p *RemoveDeletedProcessor) GetZeroLengthLeafVariantEntries() []api.Entry {
	return p.context.GetZeroLengthLeafVariantEntries()
}

type removeDeletedTaskContext struct {
	RemoveDeletedProcessorParams
	deleteStatsCount          atomic.Int64
	zeroLeafEntryElements     []api.Entry
	zeroLeafEntryElementsLock sync.Mutex
}

func newRemoveDeletedTaskContext(p *RemoveDeletedProcessorParams) *removeDeletedTaskContext {
	return &removeDeletedTaskContext{
		RemoveDeletedProcessorParams: *p,
		deleteStatsCount:             atomic.Int64{},
		zeroLeafEntryElements:        []api.Entry{},
		zeroLeafEntryElementsLock:    sync.Mutex{},
	}
}

func (r *removeDeletedTaskContext) GetDeleteStatsCount() int64 {
	return r.deleteStatsCount.Load()
}

// GetZeroLengthLeafVariantEntries returns the entries that have zero-length leaf variant entries after removal
func (r *removeDeletedTaskContext) GetZeroLengthLeafVariantEntries() []api.Entry {
	r.zeroLeafEntryElementsLock.Lock()
	defer r.zeroLeafEntryElementsLock.Unlock()
	return r.zeroLeafEntryElements
}

// Run processes the entry tree starting from e, removing leaf variant entries marked
// for deletion by the specified owner. The pool parameter should be VirtualFailFast
// to stop on first error.
// Returns the first error encountered, or nil if successful.
func (p *RemoveDeletedProcessor) Run(e api.Entry, poolFactory pool.VirtualPoolFactory) error {

	// create a virtual task pool for removeDeleted operations
	pool := poolFactory.NewVirtualPool(pool.VirtualFailFast)

	if err := pool.Submit(newRemoveDeletedTask(p.context, e, false)); err != nil {
		// Clean up pool even on early error
		pool.CloseAndWait()
		return err
	}

	// Close pool and wait for all tasks to complete before checking errors
	pool.CloseAndWait()

	// Return first error for fail-fast mode, or combined errors for tolerant mode
	return errors.Join(pool.Errors()...)
}

type removeDeletedTask struct {
	context      *removeDeletedTaskContext
	e            api.Entry
	keepDefaults bool
}

func newRemoveDeletedTask(c *removeDeletedTaskContext, e api.Entry, keepDefaults bool) *removeDeletedTask {
	return &removeDeletedTask{
		context:      c,
		e:            e,
		keepDefaults: keepDefaults,
	}
}

func (t *removeDeletedTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	res := t.e.GetLeafVariants().RemoveDeletedByOwner(t.context.Owner)
	if res != nil {
		// increment the delete stats count
		t.context.deleteStatsCount.Add(1)
	}
	if t.e.CanDeleteBranch(t.keepDefaults) {
		func() {
			t.context.zeroLeafEntryElementsLock.Lock()
			defer t.context.zeroLeafEntryElementsLock.Unlock()
			t.context.zeroLeafEntryElements = append(t.context.zeroLeafEntryElements, t.e)
		}()
		return nil
	}

	// Process children recursively
	for _, c := range t.e.GetChilds(types.DescendMethodAll) {
		childTask := newRemoveDeletedTask(t.context, c, t.e.GetSchema().GetContainer() == nil)
		// Submit may fail if pool is closed or fail-fast error occurred
		if err := submit(childTask); err != nil {
			return err
		}
	}

	return nil
}
