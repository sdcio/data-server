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

	"github.com/sdcio/data-server/pkg/tree/tree_persist"
)

type CacheClientBound interface {
	InstanceCreate(ctx context.Context) error
	InstanceDelete(ctx context.Context) error
	InstanceExists(ctx context.Context) bool
	IntentsList(ctx context.Context) ([]string, error)
	IntentGet(ctx context.Context, intentName string) (*tree_persist.Intent, error)
	IntentModify(ctx context.Context, intent *tree_persist.Intent) error
	IntentDelete(ctx context.Context, intentName string) error
	IntentExists(ctx context.Context, intentName string) (bool, error)
	IntentGetAll(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error)
	InstanceClose(ctx context.Context) error
}

type CacheClientBoundImpl struct {
	cacheClient Client
	cacheName   string
}

func NewCacheClientBound(name string, c Client) *CacheClientBoundImpl {
	return &CacheClientBoundImpl{
		cacheClient: c,
		cacheName:   name,
	}
}

func (c *CacheClientBoundImpl) InstanceCreate(ctx context.Context) error {
	return c.cacheClient.InstanceCreate(ctx, c.cacheName)
}
func (c *CacheClientBoundImpl) InstanceDelete(ctx context.Context) error {
	return c.cacheClient.InstanceDelete(ctx, c.cacheName)
}
func (c *CacheClientBoundImpl) InstanceClose(ctx context.Context) error {
	return c.cacheClient.InstanceClose(ctx, c.cacheName)
}
func (c *CacheClientBoundImpl) InstanceExists(ctx context.Context) bool {
	return c.cacheClient.InstanceExists(ctx, c.cacheName)
}
func (c *CacheClientBoundImpl) IntentsList(ctx context.Context) ([]string, error) {
	return c.cacheClient.InstanceIntentsList(ctx, c.cacheName)
}
func (c *CacheClientBoundImpl) IntentGet(ctx context.Context, intentName string) (*tree_persist.Intent, error) {
	return c.cacheClient.InstanceIntentGet(ctx, c.cacheName, intentName)
}
func (c *CacheClientBoundImpl) IntentModify(ctx context.Context, intent *tree_persist.Intent) error {
	return c.cacheClient.InstanceIntentModify(ctx, c.cacheName, intent)
}
func (c *CacheClientBoundImpl) IntentDelete(ctx context.Context, intentName string) error {
	return c.cacheClient.InstanceIntentDelete(ctx, c.cacheName, intentName)
}
func (c *CacheClientBoundImpl) IntentExists(ctx context.Context, intentName string) (bool, error) {
	return c.cacheClient.InstanceIntentExists(ctx, c.cacheName, intentName)
}
func (c *CacheClientBoundImpl) IntentGetAll(ctx context.Context, excludeIntentNames []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
	c.cacheClient.InstanceIntentGetAll(ctx, c.cacheName, excludeIntentNames, intentChan, errChan)
}
