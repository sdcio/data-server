package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type Target interface {
	Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error)
	Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error)
	Subscribe()
	//
	Sync(ctx context.Context)
}

func New(ctx context.Context, name string, cfg *config.SBI, main *ctree.Tree) (Target, error) {
	switch cfg.Type {
	case "gnmi":
		return newGNMITarget(ctx, name, cfg, main)
	case "nc":
		return newNCTarget(ctx, cfg, main)
	case "redis":
		return newRedisTarget(ctx, cfg, main)
	case "nats":
		return newNATSTarget(ctx, cfg, main)
	}
	return nil, nil
}
