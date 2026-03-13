package validation

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	sdcpbutils "github.com/sdcio/sdc-protos/utils"
)

// validateRange int and uint types (Leaf and Leaflist) define ranges which configured values must lay in.
// validateRange does check this condition.
func validateRange(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {

	// if no schema present or Field and LeafList Types do not contain any ranges, return there is nothing to check
	if e.GetSchema() == nil || (len(e.GetSchema().GetField().GetType().GetRange()) == 0 && len(e.GetSchema().GetLeaflist().GetType().GetRange()) == 0) {
		return
	}

	lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
	if lv == nil {
		return
	}

	tv := lv.Update.Value()

	var tvs []*sdcpb.TypedValue
	var typeSchema *sdcpb.SchemaLeafType
	// ranges are defined on Leafs or LeafLists.
	switch {
	case len(e.GetSchema().GetField().GetType().GetRange()) != 0:
		// if it is a leaf, extract the value add it as a single value to the tvs slice and check it further down
		tvs = []*sdcpb.TypedValue{tv}
		// we also need the Field/Leaf Type schema
		typeSchema = e.GetSchema().GetField().GetType()
	case len(e.GetSchema().GetLeaflist().GetType().GetRange()) != 0:
		// if it is a leaflist, extract the values them to the tvs slice and check them further down
		tvs = tv.GetLeaflistVal().GetElement()
		// we also need the Field/Leaf Type schema
		typeSchema = e.GetSchema().GetLeaflist().GetType()
	default:
		// if no ranges exist return
		return
	}

	// range through the tvs and check that they are in range
	for _, tv := range tvs {
		// we need to distinguish between unsigned and singned ints
		switch typeSchema.TypeName {
		case "uint8", "uint16", "uint32", "uint64":
			// procede with the unsigned ints
			urnges := sdcpbutils.NewRnges[uint64]()
			// add the defined ranges to the ranges struct
			for _, r := range typeSchema.GetRange() {
				urnges.AddRange(r.Min.Value, r.Max.Value)
			}
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))

			// check the value lays within any of the ranges
			if !urnges.IsWithinAnyRange(tv.GetUintVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", e.SdcpbPath().ToXPath(false), tv.GetUintVal(), urnges.String()), types.ValidationResultEntryTypeError)
			}

		case "int8", "int16", "int32", "int64":
			// procede with the signed ints
			srnges := sdcpbutils.NewRnges[int64]()
			for _, r := range typeSchema.GetRange() {
				// get the value
				min := int64(r.GetMin().GetValue())
				max := int64(r.GetMax().GetValue())
				// take care of the minus sign
				if r.Min.Negative {
					min = min * -1
				}
				if r.Max.Negative {
					max = max * -1
				}
				// add the defined ranges to the ranges struct
				srnges.AddRange(min, max)
			}
			stats.Add(types.StatTypeRange, uint32(len(typeSchema.GetRange())))
			// check the value lays within any of the ranges
			if !srnges.IsWithinAnyRange(tv.GetIntVal()) {
				resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("path %s, value %d not within any of the expected ranges %s", e.SdcpbPath().ToXPath(false), tv.GetIntVal(), srnges.String()), types.ValidationResultEntryTypeError)
			}
		}
	}
}
