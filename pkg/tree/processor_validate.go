package tree

import (
	"context"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

type ValidateProcessor struct {
	parameters *ValidateProcessorParameters
}

func NewValidateProcessor(parameters *ValidateProcessorParameters) *ValidateProcessor {
	return &ValidateProcessor{
		parameters: parameters,
	}
}

func (p *ValidateProcessor) Run(taskpoolFactory pool.VirtualPoolFactory, e api.Entry) {
	taskpool := taskpoolFactory.NewVirtualPool(pool.VirtualTolerant)
	taskpool.Submit(newValidateTask(e, p.parameters))
	taskpool.CloseAndWait()
}

type ValidateProcessorParameters struct {
	resultChan chan<- *types.ValidationResultEntry
	stats      *types.ValidationStats
	vCfg       *config.Validation
}

func NewValidateProcessorConfig(resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, vCfg *config.Validation) *ValidateProcessorParameters {
	return &ValidateProcessorParameters{
		resultChan: resultChan,
		stats:      stats,
		vCfg:       vCfg,
	}
}

type validateTask struct {
	e          api.Entry
	parameters *ValidateProcessorParameters
}

func newValidateTask(e api.Entry, parameters *ValidateProcessorParameters) *validateTask {
	return &validateTask{
		e:          e,
		parameters: parameters,
	}
}

func (t *validateTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ctx.Err() != nil {
		return nil
	}
	// validate the mandatory statement on this entry
	if t.e.RemainsToExist() {
		t.e.ValidateLevel(ctx, t.parameters.resultChan, t.parameters.stats, t.parameters.vCfg)
		for _, c := range t.e.GetChilds(types.DescendMethodActiveChilds) {
			submit(newValidateTask(c, t.parameters))
		}
	}
	return nil
}
