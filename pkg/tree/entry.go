package tree

import (
	"context"
	"math"

	"github.com/sdcio/data-server/pkg/tree/api"
)

const (
	KeysIndexSep       = "_"
	DefaultValuesPrio  = int32(math.MaxInt32 - 90)
	DefaultsIntentName = "default"
	RunningValuesPrio  = int32(math.MaxInt32 - 100)
	RunningIntentName  = "running"
	ReplaceValuesPrio  = int32(math.MaxInt32 - 110)
	ReplaceIntentName  = "replace"
)

// NewEntry constructor for Entries
func NewEntry(ctx context.Context, parent api.Entry, pathElemName string, tc api.TreeContext) (*sharedEntryAttributes, error) {
	// create a new sharedEntryAttributes instance
	sea, err := newSharedEntryAttributes(ctx, parent, pathElemName, tc)
	if err != nil {
		return nil, err
	}

	// add the Entry as a child to the parent Entry
	err = parent.AddChild(ctx, sea)
	return sea, err
}

type LeafVariantEntry interface {
	MarkDelete(onlyIntended bool) *api.LeafEntry
	GetEntry() api.Entry
	String() string
}

type DescendMethod int

const (
	DescendMethodAll DescendMethod = iota
	DescendMethodActiveChilds
)
