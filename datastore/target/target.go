package target

import (
	"context"
	"fmt"

	"github.com/iptecharch/schema-server/config"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type Target interface {
	Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error)
	Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error)
	Subscribe()
	//
	Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate)
}

func New(ctx context.Context, name string, cfg *config.SBI, schemaClient schemapb.SchemaServerClient, schema *schemapb.Schema) (Target, error) {
	switch cfg.Type {
	case "gnmi":
		return newGNMITarget(ctx, name, cfg)
	case "nc":
		return newNCTarget(ctx, name, cfg, schemaClient, schema)
	case "redis":
		return newRedisTarget(ctx, cfg)
	case "nats":
		return newNATSTarget(ctx, cfg)
	case "noop":
		return newNoopTarget(ctx, name)
	}
	return nil, fmt.Errorf("unknown DS target type %q", cfg.Type)
}

type SyncUpdate struct {
	Tree   string
	Update *schemapb.Notification
}
