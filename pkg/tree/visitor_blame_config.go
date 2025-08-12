package tree

import (
	"context"
	"slices"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type BlameConfigVisitor struct {
	BaseVisitor
	stack           []*sdcpb.BlameTreeElement
	includeDefaults bool
}

var _ EntryVisitor = (*BlameConfigVisitor)(nil)

func NewBlameConfigVisitor(includeDefaults bool) *BlameConfigVisitor {
	return &BlameConfigVisitor{
		stack:           []*sdcpb.BlameTreeElement{},
		includeDefaults: includeDefaults,
	}
}

func (b *BlameConfigVisitor) Visit(ctx context.Context, e Entry) error {
	name := e.PathName()
	if e.IsRoot() {
		name = "root"
	}
	result := sdcpb.NewBlameTreeElement(name)
	skipAdd := false

	// process Value
	highestLe := e.GetLeafVariantEntries().GetHighestPrecedence(false, true)
	if highestLe != nil {
		if highestLe.Update.Owner() != DefaultsIntentName || b.includeDefaults {
			result.SetValue(highestLe.Update.Value()).SetOwner(highestLe.Update.Owner())

			// check if running equals the expected
			runningLe := e.GetLeafVariantEntries().GetRunning()
			if runningLe != nil {
				if !proto.Equal(runningLe.Update.Value(), highestLe.Update.Value()) {
					result.DeviationValue = runningLe.Value()
				}
			}
		} else {
			// if it is default but no default is meant to be returned
			skipAdd = true
		}
	}
	// add to the result tree as a child
	if !skipAdd && len(b.stack) > 0 {
		b.stack[len(b.stack)-1].AddChild(result)
	}
	// add to the stack as last element
	b.stack = append(b.stack, result)
	return nil
}

func (b *BlameConfigVisitor) Up() {
	// sort to make te output stable
	slices.SortFunc(b.stack[len(b.stack)-1].Childs, func(a *sdcpb.BlameTreeElement, b *sdcpb.BlameTreeElement) int {
		return strings.Compare(a.GetName(), b.GetName())
	})
	// remove the last elem from stack
	if len(b.stack) > 1 {
		b.stack = b.stack[:len(b.stack)-1]
	}
}

func (o *BlameConfigVisitor) DescendMethod() DescendMethod {
	return DescendMethodAll
}

func (b *BlameConfigVisitor) GetResult() *sdcpb.BlameTreeElement {
	if len(b.stack) == 1 {
		return b.stack[0]
	}
	return nil
}
