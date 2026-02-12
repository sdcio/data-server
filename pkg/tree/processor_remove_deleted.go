package tree

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type RemoveDeletedProcessor struct {
	config *RemoveDeletedProcessorParameters
}

func NewRemoveDeletedProcessor(c *RemoveDeletedProcessorParameters) *RemoveDeletedProcessor {
	return &RemoveDeletedProcessor{
		config: c,
	}
}

type RemoveDeletedProcessorParameters struct {
	owner                     string
	deleteStatsCount          atomic.Int64
	zeroLeafEntryElements     []Entry
	zeroLeafEntryElementsLock sync.Mutex
}

func NewRemoveDeletedProcessorParameters(owner string) *RemoveDeletedProcessorParameters {
	return &RemoveDeletedProcessorParameters{
		owner:                     owner,
		deleteStatsCount:          atomic.Int64{},
		zeroLeafEntryElements:     []Entry{},
		zeroLeafEntryElementsLock: sync.Mutex{},
	}
}

func (r *RemoveDeletedProcessorParameters) GetDeleteStatsCount() int64 {
	return r.deleteStatsCount.Load()
}

// GetZeroLengthLeafVariantEntries returns the entries that have zero-length leaf variant entries after removal
func (r *RemoveDeletedProcessorParameters) GetZeroLengthLeafVariantEntries() []Entry {
	r.zeroLeafEntryElementsLock.Lock()
	defer r.zeroLeafEntryElementsLock.Unlock()
	return r.zeroLeafEntryElements
}

// Run processes the entry tree starting from e, removing leaf variant entries marked
// for deletion by the specified owner. The pool parameter should be VirtualFailFast
// to stop on first error.
// Returns the first error encountered, or nil if successful.
func (p *RemoveDeletedProcessor) Run(e Entry, poolFactory pool.VirtualPoolFactory) error {

	// create a virtual task pool for removeDeleted operations
	pool := poolFactory.NewVirtualPool(pool.VirtualFailFast)

	if err := pool.Submit(newRemoveDeletedTask(p.config, e, false)); err != nil {
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
	config       *RemoveDeletedProcessorParameters
	e            Entry
	keepDefaults bool
}

func newRemoveDeletedTask(c *RemoveDeletedProcessorParameters, e Entry, keepDefaults bool) *removeDeletedTask {
	return &removeDeletedTask{
		config:       c,
		e:            e,
		keepDefaults: keepDefaults,
	}
}

func (t *removeDeletedTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	res := t.e.GetLeafVariantEntries().RemoveDeletedByOwner(t.config.owner)
	if res != nil {
		// increment the delete stats count
		t.config.deleteStatsCount.Add(1)
	}
	if t.e.CanDeleteBranch(t.keepDefaults) {
		func() {
			t.config.zeroLeafEntryElementsLock.Lock()
			defer t.config.zeroLeafEntryElementsLock.Unlock()
			t.config.zeroLeafEntryElements = append(t.config.zeroLeafEntryElements, t.e)
		}()
		return nil
	}

	// Process children recursively
	for _, c := range t.e.GetChilds(types.DescendMethodAll) {
		childTask := newRemoveDeletedTask(t.config, c, t.e.GetSchema().GetContainer() == nil)
		// Submit may fail if pool is closed or fail-fast error occurred
		if err := submit(childTask); err != nil {
			return err
		}
	}

	return nil
}
