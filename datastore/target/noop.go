package target

import (
	"context"

	"github.com/iptecharch/data-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
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

func (t *noopTarget) Get(_ context.Context, _ *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	result := &schemapb.GetDataResponse{
		Notification: []*schemapb.Notification{},
	}
	return result, nil
}

func (t *noopTarget) Set(_ context.Context, _ *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	result := &schemapb.SetDataResponse{
		Response: []*schemapb.UpdateResult{},
	}
	return result, nil
}

func (t *noopTarget) Subscribe() {}

func (t *noopTarget) Sync(ctx context.Context, _ *config.Sync, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.name)
}

func (t *noopTarget) Close() {}
