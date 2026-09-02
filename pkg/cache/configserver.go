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
	"strings"
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
// are read through and written to a configserver.LocalConfigClient seam over
// the colocated config-server controller (ConfigSnapshotService) — this
// backend never persists its own copy of them, and Modify/Delete write
// through synchronously at apply time, the same moment Cache.Type: local
// persists last-applied, so last-applied never lags behind southbound apply
// (see pkg/cache/docs/adr/0003-config-server-write-path-real-last-applied-writes.md).
// "running" is unaffected by backend choice (see the ADR): it is kept in its
// own independent in-memory, per-instance store, entirely separate from the
// seam.
type ConfigServerCache struct {
	client configserver.LocalConfigClient

	mu      sync.RWMutex
	running map[string]*tree_persist.Intent
}

// NewConfigServerCache returns a Client backed by client for real Intents.
// Every InstanceIntent* call derives its target namespace/name from the
// cacheInstanceName it's given (see target), so a single ConfigServerCache
// correctly serves datastores across multiple Kubernetes namespaces.
func NewConfigServerCache(client configserver.LocalConfigClient) *ConfigServerCache {
	return &ConfigServerCache{
		client:  client,
		running: map[string]*tree_persist.Intent{},
	}
}

// NewConfigServerClient returns a client-backed *ConfigServerCache as a
// Client. ConfigServerCache satisfies IntentWriter directly now (Modify/
// Delete write through the LocalConfigClient seam to config-server), so
// unlike other Cache.Type backends this needs no noopIntentWriter
// composition.
func NewConfigServerClient(client configserver.LocalConfigClient) Client {
	return NewConfigServerCache(client)
}

// target derives the target namespace/name from cacheInstanceName, mirroring
// config-server's own <target namespace>.<target name> datastore-naming
// convention. It returns ErrMalformedDatastoreName when cacheInstanceName
// doesn't decode into a non-empty namespace and a non-empty name.
func (c *ConfigServerCache) target(cacheInstanceName string) (configserver.Target, error) {
	return splitDatastoreName(cacheInstanceName)
}

// lookupConfigName maps an owner/intent name onto the bare Config resource
// name ConfigReadService keys TargetSnapshot.Spec.Configs by. GetGVKNSN
// names ("<namespace>.<name>") are stripped when the namespace matches the
// target; a bare name is passed through unchanged so existing Get callers
// keep working.
func lookupConfigName(target configserver.Target, intentName string) string {
	prefix := target.Namespace + "."
	if rest, ok := strings.CutPrefix(intentName, prefix); ok && rest != "" {
		return rest
	}
	return intentName
}

// InstanceCreate/InstanceDelete/InstanceClose/InstanceExists/InstancesList
// only manage the independent "running" store: real intents live in
// config-server, never locally, so there is nothing else to create/delete.

// InstanceCreate/InstanceDelete error on an already-existing/missing
// instance, mirroring the error semantics of the underlying
// github.com/sdcio/cache Cache.InstanceCreate/InstanceDelete that LocalCache
// itself defers to for the same methods.
func (c *ConfigServerCache) InstanceCreate(ctx context.Context, cacheInstanceName string) error {
	if _, err := c.target(cacheInstanceName); err != nil {
		return err
	}
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
	target, err := c.target(cacheInstanceName)
	if err != nil {
		return nil, err
	}
	docs, err := c.client.List(ctx, target)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		names = append(names, d.IntentName())
	}
	return names, nil
}

// InstanceIntentGet calls Get, wrapping the result in an
// importer.ImportConfigAdapter per the ADR's field-mapping table.
func (c *ConfigServerCache) InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.ImportConfigAdapter, error) {
	target, err := c.target(cacheName)
	if err != nil {
		return nil, err
	}
	doc, err := c.client.Get(ctx, target, lookupConfigName(target, intentName))
	if err != nil {
		return nil, err
	}
	return csimporter.NewImportAdapter(doc)
}

// InstanceIntentExists calls Get and maps "not found" to (false, nil),
// matching the Client contract's existing meaning of that return; any other
// error propagates as (false, err).
func (c *ConfigServerCache) InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error) {
	target, err := c.target(cacheName)
	if err != nil {
		return false, err
	}
	_, err = c.client.Get(ctx, target, lookupConfigName(target, intentName))
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

	target, err := c.target(cacheName)
	if err != nil {
		errChan <- err
		return
	}

	docs, err := c.client.List(ctx, target)
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

// InstanceIntentModify flattens intent into a Document (see
// configserver.DocumentFromIntent) and writes it through the seam
// synchronously, at the same moment TransactionSet's apply loop calls it —
// matching Cache.Type: local's write-at-apply timing so last-applied never
// lags behind southbound apply.
func (c *ConfigServerCache) InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	target, err := c.target(cacheName)
	if err != nil {
		return err
	}
	name := lookupConfigName(target, intent.GetIntentName())
	doc, err := configserver.DocumentFromIntent(target, name, intent)
	if err != nil {
		return err
	}
	return c.client.Modify(ctx, target, doc)
}

// InstanceIntentDelete writes through the seam's Delete synchronously.
// ignoreNonExisting is accepted for interface compatibility but is always a
// no-op in practice: LocalConfigWriter.Delete is already idempotent on a
// missing name (TargetSnapshot's membership model has no tombstone to
// distinguish "already gone" from "never existed"), so there is no
// non-idempotent behavior left to gate.
func (c *ConfigServerCache) InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, ignoreNonExisting bool) error {
	target, err := c.target(cacheName)
	if err != nil {
		return err
	}
	return c.client.Delete(ctx, target, lookupConfigName(target, intentName))
}

var (
	_ IntentReader      = (*ConfigServerCache)(nil)
	_ IntentWriter      = (*ConfigServerCache)(nil)
	_ RunningStore      = (*ConfigServerCache)(nil)
	_ InstanceLifecycle = (*ConfigServerCache)(nil)
	_ Client            = (*ConfigServerCache)(nil)
)
