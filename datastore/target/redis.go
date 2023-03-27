package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type redisTarget struct{}

func newRedisTarget(ctx context.Context, cfg *config.SBI) (*redisTarget, error) {
	return &redisTarget{}, nil
}

func (t *redisTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}
func (t *redisTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	return nil, nil
}
func (t *redisTarget) Subscribe() {}
func (t *redisTarget) Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *redisTarget) Close() {}
