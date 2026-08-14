package validation

import (
	"context"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ValidationFunc is the common signature for all validation checks.
type ValidationFunc func(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats)

func activeValidators(vCfg *config.Validation) []ValidationFunc {
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
		active = append(active, validateMustStatements)
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

// effectiveFieldType returns the matched union branch for the highest-precedence leaf
// variant, or the outer schema type when no union branch was recorded. Returns
// (nil, false) when no leaf variant exists (nothing to validate).
func effectiveFieldType(e api.Entry, schemaType *sdcpb.SchemaLeafType) (*sdcpb.SchemaLeafType, bool) {
	lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
	if lv == nil {
		return nil, false
	}
	return lv.Update.EffectiveLeafType(schemaType), true
}

// resolveLeafref returns the effective leafref path and the highest-precedence leaf
// variant for an entry whose schema type is a leafref (direct or via a matched union
// branch). Handles both field and leaf-list schema nodes. Returns ok=false when no
// variant exists or the effective type carries no leafref path.
func resolveLeafref(e api.Entry) (lref string, lv *api.LeafEntry, ok bool) {
	lv = e.GetLeafVariants().GetHighestPrecedence(false, true, false)
	if lv == nil {
		return "", nil, false
	}
	if field := e.GetSchema().GetField(); field != nil {
		lref = lv.Update.EffectiveLeafType(field.GetType()).GetLeafref()
	}
	if lref == "" {
		if ll := e.GetSchema().GetLeaflist(); ll != nil {
			lref = lv.Update.EffectiveLeafType(ll.GetType()).GetLeafref()
		}
	}
	return lref, lv, lref != ""
}

// validateLevel runs all active validators on a single tree entry.
func validateLevel(ctx context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats, validators []ValidationFunc) {
	for _, validator := range validators {
		validator(ctx, e, resultChan, stats)
	}
}
