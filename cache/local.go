package cache

import (
	"context"
	"fmt"

	"github.com/iptecharch/cache/cache"
	"github.com/iptecharch/cache/config"
	"github.com/iptecharch/cache/proto/cachepb"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type localCache struct {
	c cache.Cache[*schemapb.TypedValue]
}

func NewLocalCache(cfg *config.CacheConfig) (Client, error) {
	lc := &localCache{
		c: cache.New(cfg,
			func() *schemapb.TypedValue {
				return &schemapb.TypedValue{}
			}),
	}
	err := lc.c.Init(context.TODO())
	if err != nil {
		return nil, err
	}
	return lc, nil
}

func (c *localCache) Create(ctx context.Context, name string, _ bool, _ bool) error {
	return c.c.Create(ctx, &cache.CacheInstanceConfig{
		Name:   name,
		Cached: true,
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
		if cand == candidate {
			return true, nil
		}
	}
	return false, nil
}

func (c *localCache) GetCandidates(ctx context.Context, name string) ([]string, error) {
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

func (c *localCache) CreateCandidate(ctx context.Context, name, candidate string) error {
	_, err := c.c.CreateCandidate(ctx, name, candidate)
	return err
}

func (c *localCache) Clone(ctx context.Context, name, clone string) error {
	_, err := c.c.Clone(ctx, name, clone)
	return err
}

func (c *localCache) Modify(ctx context.Context, name string, store cachepb.Store, dels [][]string, upds []Update) error {
	var cStore cache.Store
	switch store {
	case cachepb.Store_CONFIG:
	case cachepb.Store_STATE:
		cStore = cache.StoreState
	}
	//
	var err error
	for _, del := range dels {
		err = c.c.DeleteValue(ctx, name, cStore, del)
		if err != nil {
			return err
		}
	}

	for _, upd := range upds {
		tv, err := upd.Value()
		if err != nil {
			return err
		}
		err = c.c.WriteValue(ctx, name, cStore, upd.GetPath(), tv)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *localCache) Read(ctx context.Context, name string, store cachepb.Store, paths [][]string) []Update {
	ch := c.ReadCh(ctx, name, store, paths)
	var upds = make([]Update, 0, len(paths))
	for {
		select {
		case <-ctx.Done():
			return nil
		case u, ok := <-ch:
			if !ok {
				return upds
			}
			upds = append(upds, u)
		}
	}
}

func (c *localCache) ReadCh(ctx context.Context, name string, store cachepb.Store, paths [][]string) chan Update {
	var cStore cache.Store
	switch store {
	case cachepb.Store_CONFIG:
	case cachepb.Store_STATE:
		cStore = cache.StoreState
	}
	outCh := make(chan Update)
	for _, p := range paths {
		go func(p []string) { // TODO: limit num of goroutines ?
			ch, err := c.c.ReadValue(ctx, name, cStore, p)
			if err != nil {
				log.Errorf("failed to read path %v: %v", p, err)
				return
			}
			for {
				select {
				case <-ctx.Done():
					return
				case upd := <-ch:
					outCh <- &localUpdate{
						path: upd.P,
						tv:   upd.V,
					}
				}
			}
		}(p)
	}
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
		changes = append(changes, &Change{Update: &localUpdate{
			path: entry.P,
			tv:   entry.V,
		}})
	}
	return changes, nil
}

func (c *localCache) Discard(ctx context.Context, name, candidate string) error {
	return c.c.Discard(ctx, name, candidate)
}

func (c *localCache) NewUpdate(upd *schemapb.Update) (Update, error) {
	lupd := &localUpdate{
		path: utils.ToStrings(upd.GetPath(), false, false),
		tv:   upd.GetValue(),
	}
	return lupd, nil
}

func (c *localCache) Close() error {
	return c.c.Close()
}

type localUpdate struct {
	path []string
	tv   *schemapb.TypedValue
}

func (u *localUpdate) GetPath() []string {
	return u.path
}

func (u *localUpdate) Value() (*schemapb.TypedValue, error) {
	return u.tv, nil
}

func (u *localUpdate) Bytes() []byte {
	b, _ := proto.Marshal(u.tv)
	return b
}
