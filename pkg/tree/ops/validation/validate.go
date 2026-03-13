package validation

import (
	"context"
	"errors"
	"sync"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

var (
	ErrValidation = errors.New("validation error")
)

func Validate(ctx context.Context, e api.Entry, vCfg *config.Validation, taskpoolFactory pool.VirtualPoolFactory) (types.ValidationResults, *types.ValidationStats) {
	// perform validation
	// we use a channel and cumulate all the errors
	validationResultEntryChan := make(chan *types.ValidationResultEntry, 10)
	validationStats := types.NewValidationStats()

	// create a ValidationResult struct
	validationResult := types.ValidationResults{}

	syncWait := &sync.WaitGroup{}
	syncWait.Add(1)
	go func() {
		defer syncWait.Done()
		// read from the validationResult channel
		for e := range validationResultEntryChan {
			validationResult.AddEntry(e)
		}
	}()

	validationProcessor := NewValidateProcessor(NewValidateProcessorConfig(validationResultEntryChan, validationStats, vCfg))
	validationProcessor.Run(taskpoolFactory, e)
	close(validationResultEntryChan)

	syncWait.Wait()
	return validationResult, validationStats
}
