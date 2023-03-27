package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type natsTarget struct{}

func newNATSTarget(ctx context.Context, cfg *config.SBI) (*natsTarget, error) {
	return &natsTarget{}, nil
}

func (t *natsTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}
func (t *natsTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	return nil, nil
}
func (t *natsTarget) Subscribe() {}
func (t *natsTarget) Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}
func (t *natsTarget) Close() {}
