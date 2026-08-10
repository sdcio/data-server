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

package cache

import (
	"context"

	"github.com/sdcio/cache/pkg/cache"
	"github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/cache/pkg/store/filesystem"
	"github.com/sdcio/cache/pkg/types"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/proto"
)

func NewLocalCache(cfg *config.CacheConfig) (Client, error) {

	fnc, err := filesystem.PreConfigureFilesystemInitFunc(cfg.Dir)
	if err != nil {
		return nil, err
	}

	cache, err := cache.NewCache(fnc)
	if err != nil {
		return nil, err
	}

	return &LocalCache{
		Cache: cache,
	}, nil
}

type LocalCache struct {
	*cache.Cache
}

// decodeIntent unmarshals the disk-backed store's raw bytes into a
// *tree_persist.Intent and wraps it as an importer.ImportConfigAdapter — the
// shared decode step every InstanceIntentGet/InstanceIntentGetAll/
// InstanceRunningGet call site needs.
func (l *LocalCache) decodeIntent(b []byte) (importer.ImportConfigAdapter, error) {
	result := &tree_persist.Intent{}
	if err := proto.Unmarshal(b, result); err != nil {
		return nil, err
	}
	return treeproto.NewProtoTreeImporter(result), nil
}

func (l *LocalCache) InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.ImportConfigAdapter, error) {
	b, err := l.Cache.InstanceIntentGet(ctx, cacheName, intentName)
	if err != nil {
		return nil, err
	}
	return l.decodeIntent(b)
}

func (l *LocalCache) InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChanOrig chan<- importer.ImportConfigAdapter, errChanOrig chan<- error) {
	// create new channels
	intentChan := make(chan *types.Intent, 5)
	errChan := make(chan error, 1)

	go l.Cache.InstanceIntentGetAll(ctx, cacheName, excludeIntentNames, intentChan, errChan)

	defer close(intentChanOrig)
	defer close(errChanOrig)

	for {
		select {
		case <-ctx.Done(): // Or stop if context is canceled
			return
		case intent, ok := <-intentChan: // retieve intent
			if !ok {
				return
			}
			adapter, err := l.decodeIntent(intent.Data())
			if err != nil {
				errChanOrig <- err
				return
			}
			// forward to caller
			intentChanOrig <- adapter
		case err, ok := <-errChan: // Handle errors after intents
			if !ok {
				errChan = nil // Mark errChan as nil so select ignores it
				continue
			}
			errChanOrig <- err
			return
		}
	}
}

func (l *LocalCache) InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	b, err := proto.Marshal(intent)
	if err != nil {
		return err
	}
	return l.Cache.InstanceIntentModify(ctx, cacheName, intent.GetIntentName(), b)
}

// InstanceRunningGet and InstanceRunningModify are thin passthroughs to the same
// disk-backed mechanism InstanceIntentGet/InstanceIntentModify already use, so the
// local backend's on-disk behavior for "running" is unchanged by the split — it
// simply becomes unreachable via the intent-name-keyed InstanceIntent* surface.
func (l *LocalCache) InstanceRunningGet(ctx context.Context, cacheName string) (importer.ImportConfigAdapter, error) {
	b, err := l.Cache.InstanceIntentGet(ctx, cacheName, consts.RunningIntentName)
	if err != nil {
		return nil, err
	}
	return l.decodeIntent(b)
}

func (l *LocalCache) InstanceRunningModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	b, err := proto.Marshal(intent)
	if err != nil {
		return err
	}
	return l.Cache.InstanceIntentModify(ctx, cacheName, consts.RunningIntentName, b)
}
