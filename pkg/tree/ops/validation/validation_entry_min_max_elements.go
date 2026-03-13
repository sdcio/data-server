package validation

import (
	"context"
	"fmt"
	"math"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func validateMinMaxElements(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	var contSchema *sdcpb.ContainerSchema
	if contSchema = e.GetSchema().GetContainer(); contSchema == nil {
		// if it is not a container, return
		return
	}
	if len(contSchema.GetKeys()) == 0 {
		// if it is not a list, return
		return
	}

	// get all the childs, skipping the key levels
	childs, err := ops.GetListChilds(e)
	if err != nil {
		resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("error getting childs for min/max-elements check %v", err), types.ValidationResultEntryTypeError)
	}

	intMin := int(contSchema.GetMinElements())
	intMax := int(contSchema.GetMaxElements())

	// early exit if no specific min/max defined
	if intMin <= 0 && intMax <= 0 {
		return
	}

	ownersSet := map[string]struct{}{}
	for _, child := range childs {
		childAttributes := child.GetChilds(types.DescendMethodActiveChilds)
		keyName := contSchema.GetKeys()[0].GetName()
		if keyAttr, ok := childAttributes[keyName]; ok {
			highestPrec := ops.GetHighestPrecedence(keyAttr, false, false, false)
			if len(highestPrec) > 0 {
				owner := highestPrec[0].Update.Owner()
				ownersSet[owner] = struct{}{}
			}
		}
	}
	// dedup the owners
	owners := []string{}
	for k := range ownersSet {
		owners = append(owners, k)
	}

	if len(childs) < intMin {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Min-Elements violation on %s expected %d actual %d", e.SdcpbPath().ToXPath(false), intMin, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	if intMax > 0 && len(childs) > intMax {
		for _, owner := range owners {
			resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("Max-Elements violation on %s expected %d actual %d", e.SdcpbPath().ToXPath(false), intMax, len(childs)), types.ValidationResultEntryTypeError)
		}
	}
	stats.Add(types.StatTypeMinMaxElementsList, 1)
}

// validateLeafListMinMaxAttributes validates the Min-, and Max-Elements attribute of the Entry if it is a Leaflists.
func validateLeafListMinMaxAttributes(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetLeaflist(); schema != nil {
		if schema.GetMinElements() > 0 || schema.GetMaxElements() < math.MaxUint64 {
			if lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false); lv != nil {
				tv := lv.Update.Value()

				if val := tv.GetLeaflistVal(); val != nil {
					// check minelements if set
					if schema.GetMinElements() > 0 && len(val.GetElement()) < int(schema.GetMinElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d min-elements but only %d elements are present", e.SdcpbPath().ToXPath(false), schema.MinElements, len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
					// check maxelements if set
					if uint64(len(val.GetElement())) > uint64(schema.GetMaxElements()) {
						resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("leaflist %s defines %d max-elements but %d elements are present", e.SdcpbPath().ToXPath(false), schema.GetMaxElements(), len(val.GetElement())), types.ValidationResultEntryTypeError)
					}
				}
				stats.Add(types.StatTypeMinMaxElementsLeaflist, 1)
			}
		}
	}
}
