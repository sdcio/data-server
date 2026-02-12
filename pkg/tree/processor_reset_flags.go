package tree

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// ResetFlagsProcessor resets the flags on leaf variant entries
type ResetFlagsProcessor struct {
	config *ResetFlagsProcessorParameters
}

func NewResetFlagsProcessor(c *ResetFlagsProcessorParameters) *ResetFlagsProcessor {
	return &ResetFlagsProcessor{
		config: c,
	}
}

type ResetFlagsProcessorParameters struct {
	deleteFlag, newFlag, updateFlag bool
	adjustedFlagsCount              atomic.Int64
}

func NewResetFlagsProcessorParameters() *ResetFlagsProcessorParameters {
	return &ResetFlagsProcessorParameters{
		adjustedFlagsCount: atomic.Int64{},
	}
}

func (r *ResetFlagsProcessorParameters) SetDeleteFlag() *ResetFlagsProcessorParameters {
	r.deleteFlag = true
	return r
}

func (r *ResetFlagsProcessorParameters) SetNewFlag() *ResetFlagsProcessorParameters {
	r.newFlag = true
	return r
}

func (r *ResetFlagsProcessorParameters) SetUpdateFlag() *ResetFlagsProcessorParameters {
	r.updateFlag = true
	return r
}

// GetAdjustedFlagsCount returns the number of flags that were adjusted
func (r *ResetFlagsProcessorParameters) GetAdjustedFlagsCount() int64 {
	return r.adjustedFlagsCount.Load()
}

// Run processes the entry tree starting from e, resetting flags on all leaf variant entries
// according to the processor configuration. The pool parameter can be either VirtualFailFast
// (stops on first error) or VirtualTolerant (collects all errors).
// Returns the first error for fail-fast pools, or a combined error for tolerant pools.
func (p *ResetFlagsProcessor) Run(e Entry, poolFactory pool.VirtualPoolFactory) error {
	if e == nil {
		return fmt.Errorf("entry cannot be nil")
	}

	// create a virtual task pool for resetFlags operations
	pool := poolFactory.NewVirtualPool(pool.VirtualFailFast)

	// Submit root task; workers will recursively process children
	if err := pool.Submit(newResetFlagsTask(p.config, e)); err != nil {
		// Clean up pool even on early error
		pool.CloseAndWait()
		return err
	}

	// Close pool and wait for all tasks to complete before checking errors
	pool.CloseAndWait()

	// Return first error for fail-fast mode, or combined errors for tolerant mode
	return errors.Join(pool.Errors()...)
}

type resetFlagsTask struct {
	config *ResetFlagsProcessorParameters
	e      Entry
}

func newResetFlagsTask(config *ResetFlagsProcessorParameters, e Entry) *resetFlagsTask {
	return &resetFlagsTask{
		config: config,
		e:      e,
	}
}

func (t *resetFlagsTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	// Reset flags as per config
	count := t.e.GetLeafVariantEntries().ResetFlags(t.config.deleteFlag, t.config.newFlag, t.config.updateFlag)
	t.config.adjustedFlagsCount.Add(int64(count))

	// Process children recursively
	for _, c := range t.e.GetChilds(types.DescendMethodAll) {
		// Submit may fail if pool is closed or fail-fast error occurred
		if err := submit(newResetFlagsTask(t.config, c)); err != nil {
			return err
		}
	}

	return nil
}
