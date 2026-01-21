package tree

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/sdcio/data-server/pkg/pool"
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

func NewResetFlagsProcessorParameters(deleteFlag, newFlag, updateFlag bool) *ResetFlagsProcessorParameters {
	return &ResetFlagsProcessorParameters{
		deleteFlag:         deleteFlag,
		newFlag:            newFlag,
		updateFlag:         updateFlag,
		adjustedFlagsCount: atomic.Int64{},
	}
}

// GetAdjustedFlagsCount returns the number of flags that were adjusted
func (r *ResetFlagsProcessorParameters) GetAdjustedFlagsCount() int64 {
	return r.adjustedFlagsCount.Load()
}

func (o *ResetFlagsProcessor) Run(e Entry, pool pool.VirtualPoolI) error {
	if e == nil {
		return fmt.Errorf("entry cannot be nil")
	}
	err := pool.Submit(newResetFlagsTask(o.config, e))
	if err != nil {
		return err
	}
	// close pool for additional external submission
	pool.CloseForSubmit()
	// wait for the pool to run dry
	pool.Wait()

	return pool.FirstError()
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
	// reset flags as per config
	count := t.e.GetLeafVariantEntries().ResetFlags(t.config.deleteFlag, t.config.newFlag, t.config.updateFlag)
	t.config.adjustedFlagsCount.Add(int64(count))

	// process childs
	for _, c := range t.e.GetChilds(DescendMethodAll) {
		err := submit(newResetFlagsTask(t.config, c))
		if err != nil {
			return err
		}
	}

	return nil
}
