package validation

import (
	"context"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// ValidationFunc is the common signature for all validation checks.
type ValidationFunc func(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats)

func activeValidators(vCfg *config.Validation, mt MustTracer) []ValidationFunc {
	active := make([]ValidationFunc, 0, 8)
	if !vCfg.DisabledValidators.Mandatory {
		active = append(active, validateMandatory)
	}
	if !vCfg.DisabledValidators.Leafref {
		active = append(active, validateLeafRefs)
	}
	if !vCfg.DisabledValidators.LeafrefMinMaxAttributes {
		active = append(active, validateLeafListMinMaxAttributes)
	}
	if !vCfg.DisabledValidators.Pattern {
		active = append(active, validatePattern)
	}
	if !vCfg.DisabledValidators.MustStatement {
		if mt == nil {
			active = append(active, validateMustStatements)
		} else {
			active = append(active, func(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
				validateMustStatementsWithTracer(ctx, e, resultChan, stats, mt)
			})
		}
	} else if mt != nil {
		active = append(active, func(_ context.Context, e api.Entry, _ chan<- *types.ValidationResultEntry, _ *types.ValidationStats) {
			traceSkippedEntryMusts(e, mt, "must-statement validator disabled")
		})
	}
	if !vCfg.DisabledValidators.Length {
		active = append(active, validateLength)
	}
	if !vCfg.DisabledValidators.Range {
		active = append(active, validateRange)
	}
	if !vCfg.DisabledValidators.MaxElements {
		active = append(active, validateMinMaxElements)
	}
	return active
}

// validateLevel runs all active validators on a single tree entry.
func validateLevel(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, validators []ValidationFunc) {
	for _, validator := range validators {
		validator(ctx, e, resultChan, stats)
	}
}
