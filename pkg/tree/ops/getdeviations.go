package ops

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type GetDeviationConfig struct {
	ch chan<- *types.DeviationEntry
}

func NewGetDeviationConfig(ch chan<- *types.DeviationEntry) *GetDeviationConfig {
	return &GetDeviationConfig{
		ch: ch,
	}
}

func GetDeviations(ctx context.Context, e api.Entry, config *GetDeviationConfig, poolFactory pool.VirtualPoolFactory) error {
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
	config       *GetDeviationConfig
	isActiveCase bool
}

func newDeviationTask(e api.Entry, c *GetDeviationConfig, activeCase bool) *deviationTask {
	return &deviationTask{
		entry:        e,
		isActiveCase: activeCase,
		config:       c,
	}
}

func (dt *deviationTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	evalLeafvariants := true
	// if s is a presence container but has active childs, it should not be treated as a presence
	// container, hence the leafvariants should not be processed. For presence container with
	// childs the TypedValue.empty_val in the presence container is irrelevant.
	if dt.entry.GetSchema().GetContainer().GetIsPresence() && len(dt.entry.GetChilds(types.DescendMethodActiveChilds)) > 0 {
		evalLeafvariants = false
	}

	if evalLeafvariants {
		// calculate Deviation on the LeafVariants
		dt.entry.GetLeafVariants().GetDeviations(ctx, dt.config.ch, dt.isActiveCase)
	}

	// get all active childs
	activeChilds := dt.entry.GetChilds(types.DescendMethodActiveChilds)

	// iterate through all childs
	for cName, c := range dt.entry.GetChildMap().GetAll() {
		// check if c is a active child (choice / case)
		_, isActiveChild := activeChilds[cName]
		// recurse the call
		submit(newDeviationTask(c, dt.config, isActiveChild))
	}
	return nil
}
