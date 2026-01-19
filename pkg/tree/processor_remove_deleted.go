package tree

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
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
	return r.zeroLeafEntryElements
}

func (o *RemoveDeletedProcessor) Run(e Entry, pool pool.VirtualPoolI) error {
	err := pool.Submit(newRemoveDeletedTask(o.config, e, false))
	if err != nil {
		return err
	}
	// close pool for additional external submission
	pool.CloseForSubmit()
	// wait for the pool to run dry
	pool.Wait()

	return pool.FirstError()
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
	if t.e.canDeleteBranch(t.keepDefaults) {
		func() {
			t.config.zeroLeafEntryElementsLock.Lock()
			defer t.config.zeroLeafEntryElementsLock.Unlock()
			t.config.zeroLeafEntryElements = append(t.config.zeroLeafEntryElements, t.e)
		}()
		return nil
	}

	// process childs
	for _, c := range t.e.GetChilds(DescendMethodAll) {
		childTask := newRemoveDeletedTask(t.config, c, t.e.GetSchema().GetContainer() == nil)
		err := submit(childTask)
		if err != nil {
			return err
		}
	}

	return nil
}
