package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type ncTarget struct{}

func newNCTarget(ctx context.Context, cfg *config.SBI, main *ctree.Tree) (*ncTarget, error) {
	return &ncTarget{}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}
func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	return nil, nil
}
func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {}
