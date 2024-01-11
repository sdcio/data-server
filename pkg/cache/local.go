package cache

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/iptecharch/cache/pkg/cache"
	cconfig "github.com/iptecharch/cache/pkg/config"
	"github.com/iptecharch/schema-server/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/iptecharch/data-server/pkg/config"
)

const (
	maxNumParallelReaders = 10
)

type localCache struct {
	c cache.Cache
}

func NewLocalCache(cfg *config.CacheConfig) (Client, error) {
	lc := &localCache{
		c: cache.New(&cconfig.CacheConfig{
			MaxCaches: -1,
			StoreType: cfg.StoreType,
			Dir:       cfg.Dir,
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
			Path:     del,
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
			Path:     upd.GetPath(),
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

func (c *localCache) ReadCh(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) chan *Update {
	if opts == nil {
		opts = &Opts{}
	}
	outCh := make(chan *Update, 10)
	numPaths := len(paths)
	//
	numReaders := maxNumParallelReaders
	if numPaths < numReaders {
		numReaders = numPaths
	}

	wg := new(sync.WaitGroup)
	wg.Add(numReaders)

	go func() {
		wg.Wait()
		close(outCh)
	}()
	// TODO: use period
	pCh := make(chan []string, numPaths)
	for _, p := range paths {
		pCh <- p
	}
	close(pCh)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
		MAIN:
			for {
				select {
				case <-ctx.Done():
					return
				case p, ok := <-pCh:
					if !ok {
						return
					}
					ch, err := c.c.ReadValue(ctx, name, &cache.Opts{
						Store:         getStore(opts.Store),
						Path:          p,
						Owner:         opts.Owner,
						Priority:      opts.Priority,
						PriorityCount: opts.PriorityCount,
						KeysOnly:      opts.KeysOnly,
					})
					if err != nil {
						log.Errorf("failed to read path %v: %v", p, err)
						return
					}
					for {
						select {
						case <-ctx.Done():
							return
						case e, ok := <-ch:
							if !ok {
								continue MAIN // ReadValue done, go to next path
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
				}
			}
		}()
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
