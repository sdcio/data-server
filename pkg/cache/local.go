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
	"fmt"
	"sort"
	"time"

	"github.com/sdcio/cache/pkg/cache"
	"github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/schema-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type localCache struct {
	c cache.Cache
}

func NewLocalCache(cfg *config.CacheConfig) (Client, error) {
	lc := &localCache{
		c: cache.New(cfg),
	}
	err := lc.c.Init(context.TODO())
	if err != nil {
		return nil, err
	}
	return lc, nil
}

func (c *localCache) Create(ctx context.Context, name string, _ bool, _ bool) error {
	return c.c.Create(ctx, &cache.CacheInstanceConfig{
		Name: name,
	})
}

func (c *localCache) List(ctx context.Context) ([]string, error) {
	return c.c.List(ctx), nil
}

func (c *localCache) HasCandidate(ctx context.Context, name, candidate string) (bool, error) {
	cands, err := c.GetCandidates(ctx, name)
	if err != nil {
		return false, err
	}
	for _, cand := range cands {
		if cand.CandidateName == candidate {
			return true, nil
		}
	}
	return false, nil
}

func (c *localCache) GetCandidates(ctx context.Context, name string) ([]*cache.CandidateDetails, error) {
	return c.c.Candidates(ctx, name)
}

func (c *localCache) Delete(ctx context.Context, name string) error {
	return c.c.Delete(ctx, name)
}

func (c *localCache) DeleteCandidate(ctx context.Context, name, candidate string) error {
	return c.c.Delete(ctx, fmt.Sprintf("%s/%s", name, candidate))
}

func (c *localCache) Exists(ctx context.Context, name string) (bool, error) {
	return c.c.Exists(ctx, name), nil
}

func (c *localCache) CreateCandidate(ctx context.Context, name, candidate, owner string, priority int32) error {
	_, err := c.c.CreateCandidate(ctx, name, candidate, owner, priority)
	return err
}

func (c *localCache) Clone(ctx context.Context, name, clone string) error {
	_, err := c.c.Clone(ctx, name, clone)
	return err
}

func (c *localCache) Modify(ctx context.Context, name string, opts *Opts, dels [][]string, upds []*Update) error {
	if opts == nil {
		opts = &Opts{}
	}
	//
	var err error
	for _, del := range dels {
		err = c.c.DeletePrefix(ctx, name, &cache.Opts{
			Store:    getStore(opts.Store),
			Path:     [][]string{del}, // TODO:
			Owner:    opts.Owner,
			Priority: opts.Priority,
		})
		if err != nil {
			return err
		}
	}

	for _, upd := range upds {
		err = c.c.WriteValue(ctx, name, &cache.Opts{
			Store:    getStore(opts.Store),
			Path:     [][]string{upd.GetPath()},
			Owner:    opts.Owner,
			Priority: opts.Priority,
		}, upd.Bytes())
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *localCache) Read(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) []*Update {
	ch := c.ReadCh(ctx, name, opts, paths, period)
	var upds = make([]*Update, 0, len(paths))
	for {
		select {
		case <-ctx.Done():
			return nil
		case u, ok := <-ch:
			if !ok {
				sort.Slice(upds, func(i, j int) bool {
					return upds[i].ts < upds[j].ts
				})
				return upds
			}
			upds = append(upds, u)
		}
	}
}

func (c *localCache) GetIntendedKeys(ctx context.Context, name string) (chan []string, error) {
	entryCh, err := c.c.ReadIntendedKeys(ctx, name)
	outCh := make(chan []string)
	if err != nil {
		close(outCh)
		return nil, err
	}
	go func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-entryCh:
				if !ok {
					return
				}
				if e == nil {
					continue //
				}
				outCh <- e.P
			}
		}
	}()
	return outCh, nil
}

func (c *localCache) GetIntendedKeysMeta(ctx context.Context, name string) (chan *Update, error) {
	entryCh, err := c.c.ReadIntendedKeys(ctx, name)
	outCh := make(chan *Update)
	if err != nil {
		close(outCh)
		return nil, err
	}
	go func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-entryCh:
				if !ok {
					return
				}
				if e == nil {
					continue //
				}
				outCh <- &Update{
					path:     e.P,
					value:    nil,
					priority: e.Priority,
					owner:    e.Owner,
					ts:       int64(e.Timestamp),
				}
			}
		}
	}()
	return outCh, nil
}

func (c *localCache) ReadCh(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) chan *Update {
	if opts == nil {
		opts = &Opts{}
	}
	outCh := make(chan *Update, len(paths))
	go func() {
		defer close(outCh)
		ch, err := c.c.ReadValue(ctx, name, &cache.Opts{
			Store:         getStore(opts.Store),
			Path:          paths,
			Owner:         opts.Owner,
			Priority:      opts.Priority,
			PriorityCount: opts.PriorityCount,
			KeysOnly:      opts.KeysOnly,
		})
		if err != nil {
			log.Errorf("failed to read path %v: %v", paths, err)
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				if e == nil {
					continue //
				}
				outCh <- &Update{
					path:     e.P,
					value:    e.V,
					priority: e.Priority,
					owner:    e.Owner,
					ts:       int64(e.Timestamp),
				}
			}
		}
	}()
	return outCh
}

func (c *localCache) GetChanges(ctx context.Context, name, candidate string) ([]*Change, error) {
	dels, entries, err := c.c.Diff(ctx, name, candidate)
	if err != nil {
		return nil, err
	}
	changes := make([]*Change, 0, len(dels)+len(entries))
	for _, del := range dels {
		changes = append(changes, &Change{Delete: del})
	}
	for _, entry := range entries {
		changes = append(changes, &Change{Update: &Update{
			path:  entry.P,
			value: entry.V,
		}})
	}
	return changes, nil
}

func (c *localCache) Discard(ctx context.Context, name, candidate string) error {
	return c.c.Discard(ctx, name, candidate)
}

func (c *localCache) Commit(ctx context.Context, name, candidate string) error {
	return c.c.Commit(ctx, name, candidate)
}

func (c *localCache) CreatePruneID(ctx context.Context, name string, force bool) (string, error) {
	return c.c.CreatePruneID(ctx, name, force)
}

func (c *localCache) ApplyPrune(ctx context.Context, name, id string) error {
	return c.c.ApplyPrune(ctx, name, id)
}

func (c *localCache) NewUpdate(upd *sdcpb.Update) (*Update, error) {
	b, err := proto.Marshal(upd.Value)
	if err != nil {
		return nil, err
	}
	lupd := &Update{
		path:  utils.ToStrings(upd.GetPath(), false, false),
		value: b,
	}
	return lupd, nil
}

func (c *localCache) Close() error {
	return c.c.Close()
}
