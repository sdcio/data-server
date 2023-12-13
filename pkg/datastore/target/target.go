package target

import (
	"context"
	"fmt"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"google.golang.org/grpc"

	"github.com/iptecharch/data-server/pkg/config"
	"github.com/iptecharch/data-server/pkg/schema"
)

type Target interface {
	Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error)
	Set(ctx context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error)
	Subscribe()
	//
	Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate)
}

func New(ctx context.Context, name string, cfg *config.SBI, schemaClient schema.Client, schema *sdcpb.Schema, opts ...grpc.DialOption) (Target, error) {
	switch cfg.Type {
	case "gnmi":
		return newGNMITarget(ctx, name, cfg, opts...)
	case "nc":
		return newNCTarget(ctx, name, cfg, schemaClient, schema)
	case "redis":
		return newRedisTarget(ctx, cfg)
	case "nats":
		return newNATSTarget(ctx, cfg)
	case "noop", "":
		return newNoopTarget(ctx, name)
	}
	return nil, fmt.Errorf("unknown DS target type %q", cfg.Type)
}

type SyncUpdate struct {
	// identifies the store this updates needs to be written to if Sync.Validate == false
	Store string
	// The received update
	Update *sdcpb.Notification
	// if true indicates the start of cache pruning
	Start bool
	// if true and start is true indicates first sync iteration,
	// it overrides any ongoing pruning in the cache.
	Force bool
	// if true indicates the end of a sync iteration.
	// triggers the pruning on the cache side.
	End bool
}
