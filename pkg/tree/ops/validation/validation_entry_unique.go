package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func validateUnique(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	contSchema := e.GetSchema().GetContainer()
	if contSchema == nil {
		return
	}
	constraints := contSchema.GetUniqueConstraints()
	if len(constraints) == 0 {
		return
	}
	stats.Add(types.StatTypeUnique, uint32(len(constraints)))

	childs, err := ops.GetListChilds(e)
	if err != nil {
		resultChan <- types.NewValidationResultEntry("unknown", fmt.Errorf("validateUnique: GetListChilds: %w", err), types.ValidationResultEntryTypeError)
		return
	}

	// filter out instances that will not exist after the transaction
	surviving := make([]api.Entry, 0, len(childs))
	for _, c := range childs {
		if c.RemainsToExist() {
			surviving = append(surviving, c)
		}
	}

	for _, constraint := range constraints {
		elems := constraint.GetElements()

		// v1: skip multi-segment paths and warn
		hasMultiSeg := false
		for _, elem := range elems {
			if strings.Contains(elem, "/") {
				hasMultiSeg = true
				break
			}
		}
		if hasMultiSeg {
			resultChan <- types.NewValidationResultEntry(
				"unknown",
				fmt.Errorf("list %s: unique constraint %v contains multi-segment path — skipped in v1", e.SdcpbPath().ToXPath(false), elems),
				types.ValidationResultEntryTypeWarning,
			)
			continue
		}

		// Build a TypedValue tuple for each surviving instance.
		// An instance whose tuple is incomplete (missing leaf) is excluded per RFC 7950 § 7.8.3.
		type instanceTuple struct {
			entry  api.Entry
			values []*sdcpb.TypedValue
			owner  string
		}
		tuples := make([]instanceTuple, 0, len(surviving))

		for _, inst := range surviving {
			leafChilds := inst.GetChilds(types.DescendMethodActiveChilds)
			tuple := make([]*sdcpb.TypedValue, 0, len(elems))
			complete := true

			for _, elemName := range elems {
				leafEntry, exists := leafChilds[elemName]
				if !exists || leafEntry.GetLeafVariants() == nil {
					complete = false
					break
				}
				le := leafEntry.GetLeafVariants().GetHighestPrecedence(false, false, false)
				if le == nil {
					complete = false
					break
				}
				tv := le.Update.Value()
				if tv == nil {
					complete = false
					break
				}
				tuple = append(tuple, tv)
			}

			if !complete {
				continue
			}

			// Derive the owner from the key leaf's highest-precedence variant.
			owner := ownerFromKeyLeaf(inst, contSchema)
			tuples = append(tuples, instanceTuple{entry: inst, values: tuple, owner: owner})
		}

		// O(n²) pairwise collision check.
		for i := 0; i < len(tuples); i++ {
			for j := i + 1; j < len(tuples); j++ {
				a, b := tuples[i], tuples[j]
				if tuplesEqual(a.values, b.values) {
					msg := collisionMessage(e, a.entry, b.entry, elems, a.values)
					resultChan <- types.NewValidationResultEntry(a.owner, fmt.Errorf("%s", msg), types.ValidationResultEntryTypeError)
					resultChan <- types.NewValidationResultEntry(b.owner, fmt.Errorf("%s", msg), types.ValidationResultEntryTypeError)
				}
			}
		}
	}
}

// ownerFromKeyLeaf reads the owner from the first key leaf's highest-precedence variant.
func ownerFromKeyLeaf(inst api.Entry, contSchema *sdcpb.ContainerSchema) string {
	keys := contSchema.GetKeys()
	if len(keys) == 0 {
		return "unknown"
	}
	leafChilds := inst.GetChilds(types.DescendMethodActiveChilds)
	keyLeaf, exists := leafChilds[keys[0].GetName()]
	if !exists {
		return "unknown"
	}
	le := keyLeaf.GetLeafVariants().GetHighestPrecedence(false, false, false)
	if le == nil {
		return "unknown"
	}
	return le.Update.Owner()
}

func tuplesEqual(a, b []*sdcpb.TypedValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func collisionMessage(listEntry, a, b api.Entry, elems []string, vals []*sdcpb.TypedValue) string {
	valParts := make([]string, 0, len(elems))
	for i, elem := range elems {
		valParts = append(valParts, fmt.Sprintf("%s=%v", elem, vals[i]))
	}
	return fmt.Sprintf(
		"list %s: unique constraint %v violated — entries %q and %q share the same values (%s)",
		listEntry.SdcpbPath().ToXPath(false),
		elems,
		a.SdcpbPath().ToXPath(false),
		b.SdcpbPath().ToXPath(false),
		strings.Join(valParts, ", "),
	)
}
