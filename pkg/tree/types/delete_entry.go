package types

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeleteEntry interface {
	SdcpbPath() *sdcpb.Path
}

// DeleteEntryImpl is a crutch to flag oldbestcases if on a choice, the active case changed
type DeleteEntryImpl struct {
	sdcpbPath *sdcpb.Path
}

func NewDeleteEntryImpl(sdcpbPath *sdcpb.Path) *DeleteEntryImpl {
	return &DeleteEntryImpl{
		sdcpbPath: sdcpbPath,
	}
}

func (d *DeleteEntryImpl) SdcpbPath() *sdcpb.Path {
	return d.sdcpbPath
}

type DeleteEntriesList []DeleteEntry

func (d DeleteEntriesList) SdcpbPaths() sdcpb.Paths {
	result := make([]*sdcpb.Path, 0, len(d))
	for _, del := range d {
		result = append(result, del.SdcpbPath())
	}
	return result
}
