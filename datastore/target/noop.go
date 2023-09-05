package target

import (
	"context"
	"time"

	"github.com/iptecharch/data-server/config"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type noopTarget struct {
	name string
}

func newNoopTarget(_ context.Context, name string) (*noopTarget, error) {
	nt := &noopTarget{
		name: name,
	}
	return nt, nil
}

func (t *noopTarget) Get(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	result := &sdcpb.GetDataResponse{
		Notification: make([]*sdcpb.Notification, 0, len(req.GetPath())),
	}
	for _, p := range req.GetPath() {
		result.Notification = append(result.Notification, &sdcpb.Notification{
			Timestamp: time.Now().UnixNano(),
			Update: []*sdcpb.Update{
				{
					Path: p,
				},
			},
		})
	}

	return result, nil
}

func (t *noopTarget) Set(_ context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {
	result := &sdcpb.SetDataResponse{
		Response: make([]*sdcpb.UpdateResult, 0,
			len(req.GetUpdate())+len(req.GetReplace())+len(req.GetDelete())),
		Timestamp: time.Now().UnixNano(),
	}

	for _, upd := range req.GetUpdate() {
		result.Response = append(result.Response, &sdcpb.UpdateResult{
			Path: upd.GetPath(),
			Op:   sdcpb.UpdateResult_UPDATE,
		})
	}
	for _, upd := range req.GetReplace() {
		result.Response = append(result.Response, &sdcpb.UpdateResult{
			Path: upd.GetPath(),
			Op:   sdcpb.UpdateResult_REPLACE,
		})
	}
	for _, p := range req.GetDelete() {
		result.Response = append(result.Response, &sdcpb.UpdateResult{
			Path: p,
			Op:   sdcpb.UpdateResult_DELETE,
		})
	}
	return result, nil
}

func (t *noopTarget) Subscribe() {}

func (t *noopTarget) Sync(ctx context.Context, _ *config.Sync, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.name)
}

func (t *noopTarget) Close() {}
