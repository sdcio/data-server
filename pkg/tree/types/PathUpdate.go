package types

import (
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type PathAndUpdate struct {
	path *sdcpb.Path
	upd  *Update
}

func NewPathAndUpdate(p *sdcpb.Path, upd *Update) *PathAndUpdate {
	return &PathAndUpdate{
		path: p,
		upd:  upd,
	}
}

func (pu *PathAndUpdate) GetPath() *sdcpb.Path {
	return pu.path
}

func (pu *PathAndUpdate) GetUpdate() *Update {
	return pu.upd
}

func (pu *PathAndUpdate) String() string {
	return fmt.Sprintf("path: %s, owner: %s, priority: %d, value: %s", pu.path.ToXPath(false), pu.GetUpdate().Owner(), pu.GetUpdate().Priority(), pu.GetUpdate().Value())
}

func (pu *PathAndUpdate) DeepCopy() *PathAndUpdate {
	return &PathAndUpdate{
		path: pu.path.DeepCopy(),
		upd:  pu.upd.DeepCopy(),
	}
}
