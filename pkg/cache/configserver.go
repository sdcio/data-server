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
	"errors"
	"fmt"
	"sync"

	"github.com/sdcio/data-server/pkg/cache/configserver"
	"github.com/sdcio/data-server/pkg/tree/importer"
	csimporter "github.com/sdcio/data-server/pkg/tree/importer/configserver"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/sdc-protos/tree_persist"
)

// ErrRunningNotFound is returned by ConfigServerCache.InstanceRunningGet when
// the instance exists but "running" was never modified yet.
var ErrRunningNotFound = errors.New("configserver cache: running not found")

// ConfigServerCache is the Cache.Type: config-server Client: real Intents
// are read-only, served through a configserver.LocalConfigReader seam over
// the colocated config-server controller's own watch-synced store — this
// backend never persists its own copy of them, and write calls on them are
// unconditional no-ops, since config-server/kube-api is the sole writer.
// "running" is unaffected by backend choice (see the ADR): it is kept in its
// own independent in-memory, per-instance store, entirely separate from the
// seam.
type ConfigServerCache struct {
	reader    configserver.LocalConfigReader
	namespace string

	mu      sync.RWMutex
	running map[string]*tree_persist.Intent
}

// NewConfigServerCache returns a Client backed by reader for real Intents.
// namespace scopes every InstanceIntent* call against reader's
// configserver.Target{Namespace, Name} shape: data-server has no per-target
// namespace of its own today, so every cache instance name is resolved
// against one deployment-wide namespace (the colocated controller's own).
func NewConfigServerCache(reader configserver.LocalConfigReader, namespace string) *ConfigServerCache {
	return &ConfigServerCache{
		reader:    reader,
		namespace: namespace,
		running:   map[string]*tree_persist.Intent{},
	}
}

// NewConfigServerClient composes a reader-backed *ConfigServerCache with the
// generic noopIntentWriter into a full Client. ConfigServerCache alone never
// implements IntentWriter (config-server/kube-api is the sole writer of real
// Intents), so this is the one seam Server.createCacheClient's config-server
// case uses to assemble s.cacheClient.
func NewConfigServerClient(reader configserver.LocalConfigReader, namespace string) Client {
	return struct {
		*ConfigServerCache
		noopIntentWriter
	}{
		ConfigServerCache: NewConfigServerCache(reader, namespace),
	}
}

func (c *ConfigServerCache) target(cacheInstanceName string) configserver.Target {
	return configserver.Target{Namespace: c.namespace, Name: cacheInstanceName}
}

// InstanceCreate/InstanceDelete/InstanceClose/InstanceExists/InstancesList
// only manage the independent "running" store: real intents live in
// config-server, never locally, so there is nothing else to create/delete.

// InstanceCreate/InstanceDelete error on an already-existing/missing
// instance, mirroring the error semantics of the underlying
// github.com/sdcio/cache Cache.InstanceCreate/InstanceDelete that LocalCache
// itself defers to for the same methods.
func (c *ConfigServerCache) InstanceCreate(ctx context.Context, cacheInstanceName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.running[cacheInstanceName]; exists {
		return fmt.Errorf("configserver cache: instance %q already exists", cacheInstanceName)
	}
	c.running[cacheInstanceName] = nil
	return nil
}

func (c *ConfigServerCache) InstanceDelete(ctx context.Context, cacheInstanceName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.running[cacheInstanceName]; !exists {
		return fmt.Errorf("configserver cache: instance %q does not exist", cacheInstanceName)
	}
	delete(c.running, cacheInstanceName)
	return nil
}

// InstanceClose removes the instance's running entry same as InstanceDelete:
// github.com/sdcio/cache's own Cache.InstanceClose (which LocalCache defers
// to) does the same — it closes the instance's store handle and then
// deletes it from the instance map, the same end state InstanceDelete
// reaches directly. There is no lighter-weight "close but keep" state to
// preserve here, since this backend's only local state for an instance is
// the running store itself.
func (c *ConfigServerCache) InstanceClose(ctx context.Context, cacheInstanceName string) error {
	return c.InstanceDelete(ctx, cacheInstanceName)
}

func (c *ConfigServerCache) InstanceExists(ctx context.Context, cacheInstanceName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.running[cacheInstanceName]
	return exists
}

func (c *ConfigServerCache) InstancesList(ctx context.Context) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, 0, len(c.running))
	for name := range c.running {
		result = append(result, name)
	}
	return result
}

// InstanceIntentsList calls List, mapped down to names only — client.List
// always returns full objects on the config-server side, so there is no
// cheaper name-only mode to prefer.
func (c *ConfigServerCache) InstanceIntentsList(ctx context.Context, cacheInstanceName string) ([]string, error) {
	docs, err := c.reader.List(ctx, c.target(cacheInstanceName))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, d.Name)
	}
	return names, nil
}

// InstanceIntentGet calls Get, wrapping the result in an
// importer.ImportConfigAdapter per the ADR's field-mapping table.
func (c *ConfigServerCache) InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.ImportConfigAdapter, error) {
	doc, err := c.reader.Get(ctx, c.target(cacheName), intentName)
	if err != nil {
		return nil, err
	}
	return csimporter.NewImportAdapter(doc)
}

// InstanceIntentExists calls Get and maps "not found" to (false, nil),
// matching the Client contract's existing meaning of that return; any other
// error propagates as (false, err).
func (c *ConfigServerCache) InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error) {
	_, err := c.reader.Get(ctx, c.target(cacheName), intentName)
	if err != nil {
		if errors.Is(err, configserver.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InstanceIntentGetAll calls List, then ranges over the results sending each
// into intentChan, closing intentChan/errChan when done or on ctx.Done() —
// mirroring LocalCache.InstanceIntentGetAll's existing shape.
//
// excludeIntentNames is accepted for interface compatibility but is always a
// no-op in practice: config-server has no "running" Config resource, so it
// can never appear in a List result to begin with.
func (c *ConfigServerCache) InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
	defer close(intentChan)
	defer close(errChan)

	docs, err := c.reader.List(ctx, c.target(cacheName))
	if err != nil {
		errChan <- err
		return
	}

	for _, d := range docs {
		adapter, err := csimporter.NewImportAdapter(d)
		if err != nil {
			errChan <- err
			return
		}
		select {
		case <-ctx.Done():
			return
		case intentChan <- adapter:
		}
	}
}

// InstanceRunningGet/InstanceRunningModify back "running" with its own
// independent in-memory, per-instance store — entirely separate from the
// seam, since "running" never touches config-server under any backend.

func (c *ConfigServerCache) InstanceRunningGet(ctx context.Context, cacheName string) (importer.ImportConfigAdapter, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	intent, exists := c.running[cacheName]
	if !exists {
		return nil, fmt.Errorf("configserver cache: instance %q does not exist", cacheName)
	}
	if intent == nil {
		return nil, ErrRunningNotFound
	}
	return treeproto.NewProtoTreeImporter(intent), nil
}

func (c *ConfigServerCache) InstanceRunningModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.running[cacheName]; !exists {
		return fmt.Errorf("configserver cache: instance %q does not exist", cacheName)
	}
	c.running[cacheName] = intent
	return nil
}

// ConfigServerCache deliberately does not satisfy IntentWriter — real-Intent
// writes are config-server/kube-api's job, not this backend's, and there is
// no method left on the type to no-op that away. Server.createCacheClient
// composes noopIntentWriter alongside *ConfigServerCache to produce a full
// Client.
var (
	_ IntentReader      = (*ConfigServerCache)(nil)
	_ RunningStore      = (*ConfigServerCache)(nil)
	_ InstanceLifecycle = (*ConfigServerCache)(nil)
)
