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
