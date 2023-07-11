package target

import (
	"context"

	"github.com/iptecharch/data-server/config"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type natsTarget struct{}

func newNATSTarget(ctx context.Context, cfg *config.SBI) (*natsTarget, error) {
	return &natsTarget{}, nil
}

func (t *natsTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	return nil, nil
}
func (t *natsTarget) Set(ctx context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {
	return nil, nil
}
func (t *natsTarget) Subscribe() {}
func (t *natsTarget) Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}
func (t *natsTarget) Close() {}
