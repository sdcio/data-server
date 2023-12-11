package cache

import (
	"context"
	"time"

	"github.com/iptecharch/cache/pkg/cache"
	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type Client interface {
	// create a cache
	Create(ctx context.Context, name string, ephemeral bool, cached bool) error
	// list caches
	List(ctx context.Context) ([]string, error)
	// delete cache or cache candidate
	Delete(ctx context.Context, name string) error
	// check if cache instance exists
	Exists(ctx context.Context, name string) (bool, error)
	// create a candidate
	CreateCandidate(ctx context.Context, name, candidate, owner string, priority int32) error
	// get list of candidates
	GetCandidates(ctx context.Context, name string) ([]*cache.CandidateDetails, error)
	// check if a candidate exists
	HasCandidate(ctx context.Context, name, candidate string) (bool, error)
	// deletes a candidate
	DeleteCandidate(ctx context.Context, name, candidate string) error
	// clone a cache
	Clone(ctx context.Context, name, clone string) error
	//
	CreatePruneID(ctx context.Context, name string, force bool) (string, error)
	ApplyPrune(ctx context.Context, name, id string) error
	// send a stream of modifications (update or delete) to a cache, or candidate
	Modify(ctx context.Context, name string, opts *Opts, dels [][]string, upds []*Update) error
	// read from a cache or candidate
	Read(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) []*Update
	// read from a cache or candidate, get results through a channel
	ReadCh(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) chan *Update
	// get changes present in a candidate
	GetChanges(ctx context.Context, name, candidate string) ([]*Change, error)
	// discard changes made to a candidate
	Discard(ctx context.Context, name, candidate string) error
	// commit a candidate changes into the intended store
	Commit(ctx context.Context, name, candidate string) error
	// build a cache update from a sdcpb.Update
	NewUpdate(*sdcpb.Update) (*Update, error)
	// disconnect from the cache
	Close() error
}

type Update struct {
	path     []string
	value    []byte
	priority int32
	owner    string
	ts       int64
}

func (u *Update) GetPath() []string {
	return u.path
}

func (u *Update) Value() (*sdcpb.TypedValue, error) {
	tv := new(sdcpb.TypedValue)
	err := proto.Unmarshal(u.value, tv)
	if err != nil {
		return nil, err
	}
	return tv, nil
}

func (u *Update) Bytes() []byte {
	return u.value
}

func (u *Update) Priority() int32 {
	return u.priority
}
func (u *Update) Owner() string {
	return u.owner
}

func (u *Update) TS() int64 {
	return u.ts
}

type Change struct {
	Update *Update
	Delete []string
}

type Opts struct {
	Store         cachepb.Store
	Owner         string
	Priority      int32
	PriorityCount uint64
}

func getStore(s cachepb.Store) cache.Store {
	switch s {
	default: //case cachepb.Store_CONFIG:
		return cache.StoreConfig
	case cachepb.Store_STATE:
		return cache.StoreState
	case cachepb.Store_INTENDED:
		return cache.StoreIntended
	case cachepb.Store_METADATA:
		return cache.StoreMetadata
	}
}
