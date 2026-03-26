package processors

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// ResetFlagsProcessor resets the flags on leaf variant entries
type ResetFlagsProcessor struct {
	context *resetFlagsProcessorContext
}

func NewResetFlagsProcessor(params *ResetFlagsProcessorParams) *ResetFlagsProcessor {
	return &ResetFlagsProcessor{
		context: &resetFlagsProcessorContext{
			ResetFlagsProcessorParams: *params,
			adjustedFlagsCount:        atomic.Int64{},
		},
	}
}

// GetAdjustedFlagsCount returns the number of flags that were adjusted
func (r *ResetFlagsProcessor) GetAdjustedFlagsCount() int64 {
	return r.context.GetAdjustedFlagsCount()
}

type ResetFlagsProcessorParams struct {
	DeleteFlag, NewFlag, UpdateFlag bool
}

type resetFlagsProcessorContext struct {
	ResetFlagsProcessorParams
	adjustedFlagsCount atomic.Int64
}

func (r *ResetFlagsProcessorParams) SetDeleteFlag() *ResetFlagsProcessorParams {
	r.DeleteFlag = true
	return r
}

func (r *ResetFlagsProcessorParams) SetNewFlag() *ResetFlagsProcessorParams {
	r.NewFlag = true
	return r
}

func (r *ResetFlagsProcessorParams) SetUpdateFlag() *ResetFlagsProcessorParams {
	r.UpdateFlag = true
	return r
}

// GetAdjustedFlagsCount returns the number of flags that were adjusted
func (r *resetFlagsProcessorContext) GetAdjustedFlagsCount() int64 {
	return r.adjustedFlagsCount.Load()
}

// Run processes the entry tree starting from e, resetting flags on all leaf variant entries
// according to the processor configuration. The pool parameter can be either VirtualFailFast
// (stops on first error) or VirtualTolerant (collects all errors).
// Returns the first error for fail-fast pools, or a combined error for tolerant pools.
func (p *ResetFlagsProcessor) Run(e api.Entry, poolFactory pool.VirtualPoolFactory) error {
	if e == nil {
		return fmt.Errorf("entry cannot be nil")
	}

	// create a virtual task pool for resetFlags operations
	pool := poolFactory.NewVirtualPool(pool.VirtualFailFast)

	// Submit root task; workers will recursively process children
	if err := pool.Submit(newResetFlagsTask(p.context, e)); err != nil {
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
	context *resetFlagsProcessorContext
	e       api.Entry
}

func newResetFlagsTask(context *resetFlagsProcessorContext, e api.Entry) *resetFlagsTask {
	return &resetFlagsTask{
		context: context,
		e:       e,
	}
}

func (t *resetFlagsTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	// Reset flags as per config
	count := t.e.GetLeafVariants().ResetFlags(t.context.DeleteFlag, t.context.NewFlag, t.context.UpdateFlag)
	t.context.adjustedFlagsCount.Add(int64(count))

	// Process children recursively
	for _, c := range t.e.GetChilds(types.DescendMethodAll) {
		// Submit may fail if pool is closed or fail-fast error occurred
		if err := submit(newResetFlagsTask(t.context, c)); err != nil {
			return err
		}
	}

	return nil
}
