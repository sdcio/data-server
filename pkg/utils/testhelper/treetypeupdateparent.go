package testhelper

import sdcpb "github.com/sdcio/sdc-protos/sdcpb"

type UpdateParentMock struct {
	path *sdcpb.Path
}

func NewUpdateParentMock(p *sdcpb.Path) *UpdateParentMock {
	return &UpdateParentMock{
		path: p,
	}
}

func (p *UpdateParentMock) SdcpbPath() *sdcpb.Path {
	return p.path
}
