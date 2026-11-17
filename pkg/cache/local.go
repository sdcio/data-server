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

func (l *LocalCache) InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (*tree_persist.Intent, error) {
	b, err := l.Cache.InstanceIntentGet(ctx, cacheName, intentName)
	if err != nil {
		return nil, err
	}

	result := &tree_persist.Intent{}
	err = proto.Unmarshal(b, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (l *LocalCache) InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChanOrig chan<- *tree_persist.Intent, errChanOrig chan<- error) {
	var streamErr error
	defer func() {
		finishIntentStream(ctx, streamErr, intentChanOrig, errChanOrig)
	}()

	// Cancel before finishIntentStream so the store stream releases before any errChanOrig wait.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	intentChan := make(chan *types.Intent, 5)
	errChan := make(chan error, 1)

	go l.Cache.InstanceIntentGetAll(streamCtx, cacheName, excludeIntentNames, intentChan, errChan)

	for intent := range intentChan {
		tpIntent := &tree_persist.Intent{}
		if err := proto.Unmarshal(intent.Data(), tpIntent); err != nil {
			streamErr = err
			return
		}
		select {
		case intentChanOrig <- tpIntent:
		case <-ctx.Done():
			streamErr = ctx.Err()
			return
		}
	}
	streamErr = <-errChan
}

func finishIntentStream(ctx context.Context, err error, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
	defer close(intentChan)
	defer close(errChan)
	if err == nil {
		return
	}
	// Buffered or waiting errChan receives err even when ctx is already done.
	select {
	case errChan <- err:
		return
	default:
	}
	select {
	case errChan <- err:
	case <-ctx.Done():
	}
}

func (l *LocalCache) InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	b, err := proto.Marshal(intent)
	if err != nil {
		return err
	}
	return l.Cache.InstanceIntentModify(ctx, cacheName, intent.GetIntentName(), b)
}
