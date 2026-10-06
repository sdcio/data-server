package ops

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type GetDeviationParams struct {
	Ch chan<- *types.DeviationEntry
}

func GetDeviations(ctx context.Context, e api.Entry, config *GetDeviationParams, poolFactory pool.VirtualPoolFactory) error {
	pool := poolFactory.NewVirtualPool(pool.VirtualTolerant)
	err := pool.Submit(newDeviationTask(e, config, true))
	if err != nil {
		pool.CloseAndWait()
		return err
	}
	pool.CloseAndWait()

	// Return first error for fail-fast mode, or combined errors for tolerant mode
	if errs := pool.Errors(); len(errs) > 0 {
		return errors.Join(errs...)
	}
	return pool.FirstError()
}

type deviationTask struct {
	entry        api.Entry
	config       *GetDeviationParams
	isActiveCase bool
}

func newDeviationTask(e api.Entry, c *GetDeviationParams, activeCase bool) *deviationTask {
	return &deviationTask{
		entry:        e,
		isActiveCase: activeCase,
		config:       c,
	}
}

func (dt *deviationTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	evalLeafvariants := true

	// get all active childs
	activeChilds := dt.entry.SnapshotChilds(types.DescendMethodActiveChilds)

	// if s is a presence container but has active childs, it should not be treated as a presence
	// container, hence the leafvariants should not be processed. For presence container with
	// childs the TypedValue.empty_val in the presence container is irrelevant.
	if dt.entry.GetSchema().GetContainer().GetIsPresence() && len(activeChilds) > 0 {
		evalLeafvariants = false
	}

	if evalLeafvariants {
		// calculate Deviation on the LeafVariants
		dt.entry.GetLeafVariants().GetDeviations(ctx, dt.config.Ch, dt.isActiveCase)
	}

	// Walk every child. Choice cases that lost are still visited, but not as the active case.
	// Activity is by child name, matching the previous map lookup.
	allChilds := dt.entry.SnapshotChilds(types.DescendMethodAll)
	if len(allChilds) == 0 {
		return nil
	}
	activeNames := make(map[string]struct{}, len(activeChilds))
	for _, c := range activeChilds {
		activeNames[c.PathName()] = struct{}{}
	}
	for _, c := range allChilds {
		_, isActiveChild := activeNames[c.PathName()]
		_ = submit(newDeviationTask(c, dt.config, isActiveChild))
	}
	return nil
}
