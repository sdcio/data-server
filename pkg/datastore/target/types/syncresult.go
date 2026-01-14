package types

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type SyncResult struct {
	deltes []*sdcpb.Path
}

func NewSyncResult() *SyncResult {
	return &SyncResult{}
}

func (r *SyncResult) AddDeletes(d ...*sdcpb.Path) {
	r.deltes = append(r.deltes, d...)
}
