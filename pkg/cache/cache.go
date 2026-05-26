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

	"github.com/sdcio/sdc-protos/tree_persist"
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
