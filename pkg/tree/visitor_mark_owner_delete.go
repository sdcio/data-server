package tree

import "context"

type MarkOwnerDeleteVisitor struct {
	owner               string
	onlyIntended        bool
	leafVariantsMatched LeafVariantSlice
}

var _ EntryVisitor = (*MarkOwnerDeleteVisitor)(nil)

func NewMarkOwnerDeleteVisitor(owner string, onlyIntended bool) *MarkOwnerDeleteVisitor {
	return &MarkOwnerDeleteVisitor{
		owner:               owner,
		onlyIntended:        onlyIntended,
		leafVariantsMatched: LeafVariantSlice{},
	}
}

func (o *MarkOwnerDeleteVisitor) Visit(ctx context.Context, e Entry) error {
	le := e.GetLeafVariantEntries().MarkOwnerForDeletion(o.owner, o.onlyIntended)
	if le != nil {
		o.leafVariantsMatched = append(o.leafVariantsMatched, le)
	}
	return nil
}

// Up not required by this visitor
func (o *MarkOwnerDeleteVisitor) Up() {}

// GetHitCount returns the number of entries marked for deletion
func (o *MarkOwnerDeleteVisitor) GetHitCount() int {
	return len(o.leafVariantsMatched)
}

// GetMatches return all the altered LeafVariants
func (o *MarkOwnerDeleteVisitor) GetMatches() LeafVariantSlice {
	return o.leafVariantsMatched
}
