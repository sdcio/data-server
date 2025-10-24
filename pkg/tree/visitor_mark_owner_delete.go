package tree

import (
	"context"
	"sync"
)

type MarkOwnerDeleteVisitor struct {
	BaseVisitor
	owner               string
	onlyIntended        bool
	leafVariantsMatched LeafVariantSlice
	lvMutex             *sync.Mutex
}

var _ EntryVisitor = (*MarkOwnerDeleteVisitor)(nil)

func NewMarkOwnerDeleteVisitor(owner string, onlyIntended bool) *MarkOwnerDeleteVisitor {
	return &MarkOwnerDeleteVisitor{
		owner:               owner,
		onlyIntended:        onlyIntended,
		leafVariantsMatched: LeafVariantSlice{},
		lvMutex:             &sync.Mutex{},
	}
}

func (o *MarkOwnerDeleteVisitor) Visit(ctx context.Context, e Entry) error {
	le := e.GetLeafVariantEntries().MarkOwnerForDeletion(o.owner, o.onlyIntended)
	if le != nil {
		o.lvMutex.Lock()
		o.leafVariantsMatched = append(o.leafVariantsMatched, le)
		o.lvMutex.Unlock()
	}
	return nil
}

// GetHitCount returns the number of entries marked for deletion
func (o *MarkOwnerDeleteVisitor) GetHitCount() int {
	return len(o.leafVariantsMatched)
}

// GetMatches return all the altered LeafVariants
func (o *MarkOwnerDeleteVisitor) GetMatches() LeafVariantSlice {
	return o.leafVariantsMatched
}
