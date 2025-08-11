package tree

import (
	"context"
)

type ExplicitDeleteVisitor struct {
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

func (edv *ExplicitDeleteVisitor) Up() {
	// noop
}

func (o *ExplicitDeleteVisitor) Config() *EntryVisitorConfig {
	return NewEntryVisitorConfig().SetDescendingMethod(DescendMethodAll)
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
	result := map[string]int{}
	for intent, edv := range e {
		result[intent] = edv.GetExplicitDeleteCreationCount()
	}
	return result
}
