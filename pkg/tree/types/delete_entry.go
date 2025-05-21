package types

import (
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeleteEntry interface {
	SdcpbPath() (*sdcpb.Path, error)
	Path() PathSlice
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

func (d *DeleteEntryImpl) SdcpbPath() (*sdcpb.Path, error) {
	return d.sdcpbPath, nil
}
func (d *DeleteEntryImpl) Path() PathSlice {
	return utils.ToStrings(d.sdcpbPath, false, false)
}

type DeleteEntriesList []DeleteEntry

func (d DeleteEntriesList) PathSlices() PathSlices {
	result := make(PathSlices, 0, len(d))
	for _, del := range d {
		result = append(result, del.Path())
	}
	return result
}
