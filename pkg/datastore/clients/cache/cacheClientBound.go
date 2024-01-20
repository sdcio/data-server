package CacheClient

import (
	"context"

	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/schema-server/pkg/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"

	"github.com/iptecharch/data-server/pkg/cache"
)

type CacheClientBound struct {
	cacheClient cache.Client
	name        string
}

func NewCacheClientBound(name string, c cache.Client) *CacheClientBound {
	return &CacheClientBound{
		cacheClient: c,
		name:        name, // the datastore name
	}
}

// GetValue retrieves config value for the provided path
func (ccb *CacheClientBound) GetValue(ctx context.Context, candidateName string, path *sdcpb.Path) (*sdcpb.TypedValue, error) {
	spath, err := utils.CompletePath(nil, path)
	if err != nil {
		return nil, err
	}
	cacheupds := ccb.cacheClient.Read(ctx, ccb.name+"/"+candidateName, &cache.Opts{Store: cachepb.Store_CONFIG}, [][]string{spath}, 0)
	if len(cacheupds) == 0 {
		return nil, nil
	}
	return cacheupds[0].Value()
}
