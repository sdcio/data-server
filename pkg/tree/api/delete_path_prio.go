package api

import (
	"iter"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeletePathPrio struct {
	owner string
	prio  int32
	paths *sdcpb.PathSet
}

func NewDeletePathPrio(owner string, prio int32) *DeletePathPrio {
	return &DeletePathPrio{
		prio:  prio,
		owner: owner,
		paths: sdcpb.NewPathSet(),
	}
}

func (ddp *DeletePathPrio) DeepCopy() *DeletePathPrio {
	result := NewDeletePathPrio(ddp.GetOwner(), ddp.GetPrio())
	result.paths = ddp.paths.DeepCopy()
	return result
}

func (dpp *DeletePathPrio) GetPathSet() *sdcpb.PathSet {
	return dpp.paths
}

func (dpp *DeletePathPrio) PathItems() iter.Seq[*sdcpb.Path] {
	return dpp.paths.Items()
}

func (dpp *DeletePathPrio) GetPrio() int32 {
	return dpp.prio
}

func (dpp *DeletePathPrio) GetOwner() string {
	return dpp.owner
}
