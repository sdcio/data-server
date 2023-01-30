package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type ncTarget struct{}

func newNCTarget(ctx context.Context, cfg *config.SBI) (*ncTarget, error) {
	return &ncTarget{}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}
func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	return nil, nil
}
func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context, syncCh chan *schemapb.Notification) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {}
