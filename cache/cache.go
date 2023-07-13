package cache

import (
	"context"
	"time"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
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
	CreateCandidate(ctx context.Context, name, candidate string) error
	// get list of candidates
	GetCandidates(ctx context.Context, name string) ([]string, error)
	// check if a candidate exists
	HasCandidate(ctx context.Context, name, candidate string) (bool, error)
	// deletes a candidate
	DeleteCandidate(ctx context.Context, name, candidate string) error
	// clone a cache
	Clone(ctx context.Context, name, clone string) error
	// send a stream of modifications (update or delete) to a cache, or candidate
	Modify(ctx context.Context, name string, store cachepb.Store, dels [][]string, upds []Update) error
	// read from a cache or candidate
	Read(ctx context.Context, name string, store cachepb.Store, paths [][]string, period time.Duration) []Update
	// read from a cache or candidate, get results through a channel
	ReadCh(ctx context.Context, name string, store cachepb.Store, paths [][]string, period time.Duration) chan Update
	// get changes present in a candidate
	GetChanges(ctx context.Context, name, candidate string) ([]*Change, error)
	// discard changes made to a candidate
	Discard(ctx context.Context, name, candidate string) error
	// build a cache update from a sdcpb.Update
	NewUpdate(*sdcpb.Update) (Update, error)
	// disconnect from the cache
	Close() error
}

type Update interface {
	GetPath() []string
	Value() (*sdcpb.TypedValue, error)
	Bytes() []byte
}

type Change struct {
	Update Update
	Delete []string
}
