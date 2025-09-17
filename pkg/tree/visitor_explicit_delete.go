package tree

import (
	"context"

	"github.com/sdcio/data-server/pkg/utils"
)

type ExplicitDeleteVisitor struct {
	BaseVisitor
	owner    string
	priority int32

	// created entries for further stat calculation
	relatedLeafVariants LeafVariantSlice
}

var _ EntryVisitor = (*ExplicitDeleteVisitor)(nil)

func NewExplicitDeleteVisitor(owner string, priority int32) *ExplicitDeleteVisitor {
	return &ExplicitDeleteVisitor{
		priority:            priority,
		owner:               owner,
		relatedLeafVariants: []*LeafEntry{},
	}
}

func (edv *ExplicitDeleteVisitor) Visit(ctx context.Context, e Entry) error {
	if !e.HoldsLeafvariants() {
		return nil
	}
	le := e.GetLeafVariantEntries().GetByOwner(edv.owner)
	if le != nil {
		le.MarkExpliciteDelete()
	} else {
		le = e.GetLeafVariantEntries().AddExplicitDeleteEntry(edv.owner, edv.priority)
	}
	edv.relatedLeafVariants = append(edv.relatedLeafVariants, le)
	return nil
}

// GetExplicitDeleteCreationCount returns the amount of all the explicitDelete LeafVariants that where created.
func (edv *ExplicitDeleteVisitor) GetExplicitDeleteCreationCount() int {
	return len(edv.relatedLeafVariants)
}

// GetCreatedExplicitDeleteLeafEntries returns all the explicitDelete LeafVariants that where created.
func (edv *ExplicitDeleteVisitor) GetCreatedExplicitDeleteLeafEntries() LeafVariantSlice {
	return edv.relatedLeafVariants
}

// ExplicitDeleteVisitors map of *ExplicitDeleteVisitor indexed by the intent name
type ExplicitDeleteVisitors map[string]*ExplicitDeleteVisitor

func (e ExplicitDeleteVisitors) Stats() map[string]int {
	return utils.MapApplyFuncToMap(e, func(k string, v *ExplicitDeleteVisitor) int {
		return v.GetExplicitDeleteCreationCount()
	})
}
