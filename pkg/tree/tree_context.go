package tree

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/pkg/cache"
)

type TreeContext struct {
	root                  Entry                    // the trees root element
	IntendedStoreIndex    map[string]UpdateSlice   // contains the keys that the intended store holds in the cache
	RunningStoreIndex     map[string]*cache.Update // contains the keys of the running config
	treeSchemaCacheClient TreeSchemaCacheClient
	actualOwner           string
}

func NewTreeContext(tscc TreeSchemaCacheClient, actualOwner string) *TreeContext {
	return &TreeContext{
		treeSchemaCacheClient: tscc,
		actualOwner:           actualOwner,
	}
}

func (t *TreeContext) SetRoot(e Entry) error {
	if t.root != nil {
		return fmt.Errorf("trying to set treecontexts root, although it is already set")
	}
	t.root = e
	return nil
}

func (t *TreeContext) GetActualOwner() string {
	return t.actualOwner
}

func (t *TreeContext) PathExists(path []string) bool {
	_, exists := t.IntendedStoreIndex[strings.Join(path, KeysIndexSep)]
	return exists
}

func (t *TreeContext) GetBranchesHighesPrecedence(path []string, filters ...CacheUpdateFilter) int32 {
	result := int32(math.MaxInt32)
	pathKey := strings.Join(path, KeysIndexSep)

	// TODO: Improve this, since it is probably an expensive operation
	for key, entries := range t.IntendedStoreIndex {
		if strings.HasPrefix(key, pathKey) {
			if prio := entries.GetLowestPriorityValue(filters); prio < result {
				result = prio
			}
		}
	}
	return result
}

func (tc *TreeContext) ReadCurrentUpdatesHighestPriorities(ctx context.Context, ccp PathSlices, count uint64) UpdateSlice {
	return tc.treeSchemaCacheClient.Read(ctx, &cache.Opts{
		Store:         cachepb.Store_INTENDED,
		PriorityCount: count,
	}, ccp.ToStringSlice())
}

func (t *TreeContext) GetPathsOfOwner(owner string) *PathSet {
	p := NewPathSet()
	for _, keyMeta := range t.IntendedStoreIndex {
		for _, k := range keyMeta {
			if k.Owner() == owner {
				// if the key is not yet listed in the keys slice, add it otherwise skip
				p.AddPath(k.GetPath())
			}
		}
	}
	return p
}

func (t *TreeContext) SetStoreIndex(si map[string]UpdateSlice) {
	slog.Debug("setting intended store index", slog.Int("length", len(si)))
	t.IntendedStoreIndex = si
}

// ReadRunning reads the value from running if the value does not exist, nil is returned
func (t *TreeContext) ReadRunning(ctx context.Context, path PathSlice) (*cache.Update, error) {
	// check if the value exists in running
	_, exists := t.RunningStoreIndex[strings.Join(path, KeysIndexSep)]
	if !exists {
		return nil, nil
	}

	updates := t.treeSchemaCacheClient.Read(ctx, &cache.Opts{
		Store:         cachepb.Store_CONFIG,
		PriorityCount: 1,
	}, [][]string{path})

	return updates[0], nil
}

// ReadRunning reads the value from running if the value does not exist, nil is returned
func (t *TreeContext) ReadRunningFull(ctx context.Context) ([]*cache.Update, error) {
	updates := t.treeSchemaCacheClient.Read(ctx, &cache.Opts{
		Store: cachepb.Store_CONFIG,
	}, [][]string{{}})

	return updates, nil
}
