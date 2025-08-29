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

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/proto"
)

type Client interface {
	InstanceCreate(ctx context.Context, cacheInstanceName string) error
	InstanceDelete(ctx context.Context, cacheInstanceName string) error
	InstanceClose(ctx context.Context, cacheInstanceName string) error
	InstanceExists(ctx context.Context, cacheInstanceName string) bool
	InstancesList(ctx context.Context) []string
	InstanceIntentsList(ctx context.Context, cacheInstanceName string) ([]string, error)
	InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (*tree_persist.Intent, error)
	InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
	InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error
	InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error)
	InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error)
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

func (u *Update) String() string {
	val := "error decoding value"
	v, err := u.Value()
	if err == nil {
		// if no error occured we use the value
		val = v.String()
	}
	return fmt.Sprintf("path: %s, owner: %s, priority: %d, value: %s", u.path, u.owner, u.priority, val)
}
