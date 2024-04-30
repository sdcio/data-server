// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package CacheClient

import (
	"context"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/schema-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/cache"
)

type CacheClientBoundImpl struct {
	cacheClient cache.Client
	name        string
}

type CacheClientBound interface {
	// GetValue retrieves config value for the provided path
	GetValue(ctx context.Context, candidateName string, path *sdcpb.Path) (*sdcpb.TypedValue, error)
	// GetValues retrieves config value from the provided path. If path is not a leaf path, all the sub paths will be returned.
	GetValues(ctx context.Context, candidateName string, path *sdcpb.Path) ([]*sdcpb.TypedValue, error)
}

func NewCacheClientBound(name string, c cache.Client) *CacheClientBoundImpl {
	return &CacheClientBoundImpl{
		cacheClient: c,
		name:        name, // the datastore name
	}
}

// GetValue retrieves config value for the provided path
func (ccb *CacheClientBoundImpl) GetValue(ctx context.Context, candidateName string, path *sdcpb.Path) (*sdcpb.TypedValue, error) {
	cacheupds, err := ccb.getValues(ctx, candidateName, path)
	if err != nil {
		return nil, err
	}
	if len(cacheupds) == 0 {
		return nil, nil
	}
	return cacheupds[0].Value()
}

// GetValues retrieves config value from the provided path. If path is not a leaf path, all the sub paths will be returned.
func (ccb *CacheClientBoundImpl) GetValues(ctx context.Context, candidateName string, path *sdcpb.Path) ([]*sdcpb.TypedValue, error) {
	cacheupds, err := ccb.getValues(ctx, candidateName, path)
	if err != nil {
		return nil, err
	}

	result := make([]*sdcpb.TypedValue, 0, len(cacheupds))

	// collect the cachupdate Values to return them
	for _, c := range cacheupds {
		val, err := c.Value()
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}

	return result, nil
}

// getValues internal function that retrieves config value for the provided path, with its sub-paths
func (ccb *CacheClientBoundImpl) getValues(ctx context.Context, candidateName string, path *sdcpb.Path) ([]*cache.Update, error) {
	spath, err := utils.CompletePath(nil, path)
	if err != nil {
		return nil, err
	}
	cacheupds := ccb.cacheClient.Read(ctx, ccb.name+"/"+candidateName, &cache.Opts{Store: cachepb.Store_CONFIG}, [][]string{spath}, 0)
	if len(cacheupds) == 0 {
		return nil, nil
	}
	return cacheupds, nil
}
