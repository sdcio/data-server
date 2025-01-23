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

package datastore

import (
	"context"
	"strings"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (d *Datastore) populateTreeWithRunning(ctx context.Context, tc *tree.TreeContext, r *tree.RootEntry) error {
	upds, err := tc.GetTreeSchemaCacheClient().ReadRunningFull(ctx)
	if err != nil {
		return err
	}

	flags := tree.NewUpdateInsertFlags()

	for _, upd := range upds {
		newUpd := cache.NewUpdate(upd.GetPath(), upd.Bytes(), tree.RunningValuesPrio, tree.RunningIntentName, 0)
		_, err := r.AddCacheUpdateRecursive(ctx, newUpd, flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func pathIsKeyAsLeaf(p *sdcpb.Path) bool {
	numPElem := len(p.GetElem())
	if numPElem < 2 {
		return false
	}

	_, ok := p.GetElem()[numPElem-2].GetKey()[p.GetElem()[numPElem-1].GetName()]
	return ok
}

func (d *Datastore) readStoreKeysMeta(ctx context.Context, store cachepb.Store) (map[string]tree.UpdateSlice, error) {
	entryCh, err := d.cacheClient.GetKeys(ctx, d.config.Name, store)
	if err != nil {
		return nil, err
	}

	result := map[string]tree.UpdateSlice{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-entryCh:
			if !ok {
				return result, nil
			}
			key := strings.Join(e.GetPath(), tree.KeysIndexSep)
			_, exists := result[key]
			if !exists {
				result[key] = tree.UpdateSlice{}
			}
			result[key] = append(result[key], e)
		}
	}
}
