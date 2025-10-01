package tree

import (
	"context"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type BlameConfigVisitorParallel struct {
	BaseParallelVisitor[sdcpb.BlameTreeElement]
	includeDefaults bool
}

var _ ParallelVisitor[sdcpb.BlameTreeElement] = (*BlameConfigVisitorParallel)(nil)

func NewBlameConfigVisitorParallel(includeDefaults bool) *BlameConfigVisitorParallel {
	return &BlameConfigVisitorParallel{
		includeDefaults: includeDefaults,
	}
}

func (v *BlameConfigVisitorParallel) DescendMethod() DescendMethod {
	return DescendMethodActiveChilds
}
func (v *BlameConfigVisitorParallel) NewNode(e Entry) *sdcpb.BlameTreeElement {
	name := e.PathName()
	if e.IsRoot() {
		name = "root"
	}
	return &sdcpb.BlameTreeElement{
		Name: name,
	}
}

// Compute returns bool if the node should be kept or removed from the result Tree
func (v *BlameConfigVisitorParallel) Compute(ctx context.Context, e Entry, n *sdcpb.BlameTreeElement) (bool, error) {
	// process Value
	highestLe := e.GetLeafVariantEntries().GetHighestPrecedence(false, true, true)
	if highestLe != nil {
		if highestLe.Update.Owner() != DefaultsIntentName || v.includeDefaults {
			n.SetValue(highestLe.Update.Value()).SetOwner(highestLe.Update.Owner())

			// check if running equals the expected
			runningLe := e.GetLeafVariantEntries().GetRunning()
			if runningLe != nil {
				if !proto.Equal(runningLe.Update.Value(), highestLe.Update.Value()) {
					n.SetDeviationValue(runningLe.Value())
				}
			}
		} else {
			// if it is default but no default is meant to be returned
			return false, nil
		}
	}
	return true, nil
}

func (v *BlameConfigVisitorParallel) Attach(parent *sdcpb.BlameTreeElement, child *sdcpb.BlameTreeElement) {
	parent.Childs = append(parent.Childs, child)
}

func (v *BlameConfigVisitorParallel) Detach(parent *sdcpb.BlameTreeElement, child *sdcpb.BlameTreeElement) {
	for idx, c := range parent.Childs {
		if c == child {
			parent.Childs = append(parent.Childs[:idx], parent.Childs[idx+1:]...)
			break
		}
	}
}
