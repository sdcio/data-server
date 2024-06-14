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
	"bytes"
	"context"
	"time"

	"github.com/sdcio/cache/pkg/cache"
	"github.com/sdcio/cache/proto/cachepb"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type Client interface {
	// Create a cache
	Create(ctx context.Context, name string, ephemeral bool, cached bool) error
	// List caches
	List(ctx context.Context) ([]string, error)
	// Delete delete cache or cache candidate
	Delete(ctx context.Context, name string) error
	// Exists check if cache instance exists
	Exists(ctx context.Context, name string) (bool, error)
	// CreateCandidate create a candidate
	CreateCandidate(ctx context.Context, name, candidate, owner string, priority int32) error
	// GetCandidates get list of candidates
	GetCandidates(ctx context.Context, name string) ([]*cache.CandidateDetails, error)
	// HasCandidate check if a candidate exists
	HasCandidate(ctx context.Context, name, candidate string) (bool, error)
	// DeleteCandidate deletes a candidate
	DeleteCandidate(ctx context.Context, name, candidate string) error
	// Clone a cache
	Clone(ctx context.Context, name, clone string) error
	// CreatePruneID
	CreatePruneID(ctx context.Context, name string, force bool) (string, error)
	// ApplyPrune
	ApplyPrune(ctx context.Context, name, id string) error
	// Modify send a stream of modifications (update or delete) to a cache, or candidate
	Modify(ctx context.Context, name string, opts *Opts, dels [][]string, upds []*Update) error
	// Read from a cache or candidate
	Read(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) []*Update
	// ReadCh read from a cache or candidate, get results through a channel
	ReadCh(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) chan *Update
	// GetChanges present in a candidate
	GetChanges(ctx context.Context, name, candidate string) ([]*Change, error)
	// Discard changes made to a candidate
	Discard(ctx context.Context, name, candidate string) error
	// Commit a candidate changes into the intended store
	Commit(ctx context.Context, name, candidate string) error
	// NewUpdate build a cache update from a sdcpb.Update
	NewUpdate(*sdcpb.Update) (*Update, error)
	// GetKeys retrieve the Keys of the specified store
	GetKeys(ctx context.Context, name string, store cachepb.Store) (chan *Update, error)
	// disconnect from the cache
	Close() error
}

type KeyData struct {
	Path    []string
	Intents []IntentMeta
}

type IntentMeta struct {
	Owner    string
	Priority int32
	Ts       int64
}

type Update struct {
	path     []string
	value    []byte
	priority int32
	owner    string
	ts       int64
}

func NewUpdate(path []string, value []byte, priority int32, owner string, ts int64) *Update {
	return &Update{
		path:     path,
		value:    value,
		priority: priority,
		owner:    owner,
		ts:       ts,
	}
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

// EqualSkipPath checks the equality of two updates.
// It however skips comparing paths and timestamps.
// This is a shortcut for performace, for cases in which it is already clear that the path is definately equal.
func (u *Update) EqualSkipPath(other *Update) bool {
	return u.owner == other.owner && u.priority == other.priority && bytes.Equal(u.value, other.value)
}

type Change struct {
	Update *Update
	Delete []string
}

type Opts struct {
	Store         cachepb.Store
	Owner         string // represents the intent name
	Priority      int32
	PriorityCount uint64
	KeysOnly      bool
}

func getStore(s cachepb.Store) cache.Store {
	switch s {
	default: //case cachepb.Store_CONFIG:
		return cache.StoreConfig
	case cachepb.Store_STATE:
		return cache.StoreState
	case cachepb.Store_INTENDED:
		return cache.StoreIntended
	case cachepb.Store_INTENTS:
		return cache.StoreIntents
	}
}
