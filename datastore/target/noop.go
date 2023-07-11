package target

import (
	"context"

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

func (t *noopTarget) Get(_ context.Context, _ *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	result := &sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{},
	}
	return result, nil
}

func (t *noopTarget) Set(_ context.Context, _ *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {
	result := &sdcpb.SetDataResponse{
		Response: []*sdcpb.UpdateResult{},
	}
	return result, nil
}

func (t *noopTarget) Subscribe() {}

func (t *noopTarget) Sync(ctx context.Context, _ *config.Sync, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.name)
}

func (t *noopTarget) Close() {}
