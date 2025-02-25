package tree

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/pkg/cache"
)

type TreeCacheClient interface {
	// RefreshCaches refresh the running and intended Index cache
	RefreshCaches(ctx context.Context) error

	// CACHE based Functions
	// ReadIntended retrieves the highes priority value from the intended store
	Read(ctx context.Context, opts *cache.Opts, paths [][]string) []*cache.Update

	ReadRunningPath(ctx context.Context, path PathSlice) (*cache.Update, error)
	ReadRunningFull(ctx context.Context) ([]*cache.Update, error)
	GetBranchesHighestPrecedence(ctx context.Context, path []string, filters ...CacheUpdateFilter) int32
	ReadCurrentUpdatesHighestPriorities(ctx context.Context, ccp PathSlices, count uint64) UpdateSlice
	IntendedPathExists(ctx context.Context, path []string) (bool, error)
	ReadUpdatesOwner(ctx context.Context, owner string) UpdateSlice
}

type TreeCacheClientImpl struct {
	cc        cache.Client
	datastore string

	timeout time.Duration

	intendedStoreIndex      map[string]UpdateSlice // contains the keys that the intended store holds in the cache
	intendedStoreIndexMutex sync.RWMutex
	runningStoreIndex       map[string]UpdateSlice // contains the keys of the running config
	runningStoreIndexMutex  sync.RWMutex
}

func NewTreeCacheClient(datastore string, cc cache.Client) *TreeCacheClientImpl {
	return &TreeCacheClientImpl{
		cc:                      cc,
		datastore:               datastore,
		timeout:                 time.Second * 2,
		intendedStoreIndexMutex: sync.RWMutex{},
		runningStoreIndexMutex:  sync.RWMutex{},
	}
}

func (t *TreeCacheClientImpl) IntendedPathExists(ctx context.Context, path []string) (bool, error) {
	t.intendedStoreIndexMutex.RLock()
	if t.intendedStoreIndex == nil {
		t.intendedStoreIndexMutex.RUnlock()
		t.RefreshCaches(ctx)
		t.intendedStoreIndexMutex.RLock()
	}
	defer t.intendedStoreIndexMutex.RUnlock()
	_, exists := t.intendedStoreIndex[strings.Join(path, KeysIndexSep)]
	return exists, nil
}

func (c *TreeCacheClientImpl) Read(ctx context.Context, opts *cache.Opts, paths [][]string) []*cache.Update {
	if opts == nil {
		opts = &cache.Opts{
			PriorityCount: 1,
		}
	}

	return c.cc.Read(ctx, c.datastore, opts, paths, c.timeout)
}

func (c *TreeCacheClientImpl) RefreshCaches(ctx context.Context) error {

	var err error
	c.runningStoreIndexMutex.Lock()
	c.runningStoreIndex, err = c.readStoreKeysMeta(ctx, cachepb.Store_CONFIG)
	c.runningStoreIndexMutex.Unlock()
	if err != nil {
		return err
	}
	c.intendedStoreIndexMutex.Lock()
	c.intendedStoreIndex, err = c.readStoreKeysMeta(ctx, cachepb.Store_INTENDED)
	c.intendedStoreIndexMutex.Unlock()
	if err != nil {
		return err
	}
	return nil
}

func (c *TreeCacheClientImpl) readStoreKeysMeta(ctx context.Context, store cachepb.Store) (map[string]UpdateSlice, error) {
	entryCh, err := c.cc.GetKeys(ctx, c.datastore, store)
	if err != nil {
		return nil, err
	}

	result := map[string]UpdateSlice{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-entryCh:
			if !ok {
				return result, nil
			}
			key := strings.Join(e.GetPath(), KeysIndexSep)
			_, exists := result[key]
			if !exists {
				result[key] = UpdateSlice{}
			}
			result[key] = append(result[key], e)
		}
	}
}

func (c *TreeCacheClientImpl) GetBranchesHighestPrecedence(ctx context.Context, path []string, filters ...CacheUpdateFilter) int32 {
	result := int32(math.MaxInt32)
	pathKey := strings.Join(path, KeysIndexSep)
	c.intendedStoreIndexMutex.RLock()
	if c.intendedStoreIndex == nil {
		c.intendedStoreIndexMutex.RUnlock()
		c.RefreshCaches(ctx)
		c.intendedStoreIndexMutex.RLock()
	}
	defer c.intendedStoreIndexMutex.RUnlock()

	// TODO: Improve this, since it is probably an expensive operation
	for key, entries := range c.intendedStoreIndex {
		if strings.HasPrefix(key, pathKey) {
			if prio := entries.GetLowestPriorityValue(filters); prio < result {
				result = prio
			}
		}
	}
	return result
}

func (c *TreeCacheClientImpl) ReadCurrentUpdatesHighestPriorities(ctx context.Context, ccp PathSlices, count uint64) UpdateSlice {
	return c.Read(ctx, &cache.Opts{
		Store:         cachepb.Store_INTENDED,
		PriorityCount: count,
	}, ccp.ToStringSlice())
}

func (c *TreeCacheClientImpl) ReadUpdatesOwner(ctx context.Context, owner string) UpdateSlice {

	ownerPaths := c.getPathsOfOwner(ctx, owner)

	return c.Read(ctx, &cache.Opts{
		Store: cachepb.Store_INTENDED,
		Owner: owner,
	}, ownerPaths.paths.ToStringSlice())
}

func (c *TreeCacheClientImpl) getPathsOfOwner(ctx context.Context, owner string) *PathSet {
	if c.intendedStoreIndex == nil {
		c.RefreshCaches(ctx)
	}

	p := NewPathSet()
	for _, keyMeta := range c.intendedStoreIndex {
		for _, k := range keyMeta {
			if k.Owner() == owner {
				// if the key is not yet listed in the keys slice, add it otherwise skip
				p.AddPath(k.GetPath())
			}
		}
	}
	return p
}

// ReadRunning reads the value from running if the value does not exist, nil is returned
func (c *TreeCacheClientImpl) ReadRunningPath(ctx context.Context, path PathSlice) (*cache.Update, error) {
	c.runningStoreIndexMutex.RLock()
	if c.runningStoreIndex == nil {
		c.runningStoreIndexMutex.RUnlock()
		c.RefreshCaches(ctx)
		c.runningStoreIndexMutex.RLock()
	}
	defer c.runningStoreIndexMutex.RUnlock()
	// check if the value exists in running
	_, exists := c.runningStoreIndex[strings.Join(path, KeysIndexSep)]
	if !exists {
		return nil, nil
	}

	updates := c.Read(ctx, &cache.Opts{
		Store:         cachepb.Store_CONFIG,
		PriorityCount: 1,
	}, [][]string{path})

	return updates[0], nil
}

// ReadRunning reads the value from running if the value does not exist, nil is returned
func (c *TreeCacheClientImpl) ReadRunningFull(ctx context.Context) ([]*cache.Update, error) {
	updates := c.Read(ctx, &cache.Opts{
		Store: cachepb.Store_CONFIG,
	}, [][]string{{}})

	return updates, nil
}
