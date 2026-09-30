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

package schemaClient

import (
	"context"
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/schema"
)

// tripleKey identifies a YANG schema identity by name, vendor and version.
// It is the key the registry pools SchemaClientBoundImpl instances by.
type tripleKey struct {
	Name    string
	Vendor  string
	Version string
}

func tripleKeyFromConfig(cfg *config.SchemaConfig) tripleKey {
	return tripleKey{
		Name:    cfg.Name,
		Vendor:  cfg.Vendor,
		Version: cfg.Version,
	}
}

// pooledEntry wraps a pooled *SchemaClientBoundImpl together with the number
// of datastores currently holding a handle to it.
type pooledEntry struct {
	instance *SchemaClientBoundImpl
	refcount int
}

// Registry is a process-wide pool of SchemaClientBoundImpl instances, keyed by
// schema identity (name, vendor, version) and shared/refcounted across every
// datastore using that identity. It is the "schema client registry" from the
// schema caching glossary.
//
// A Registry is safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	sc      schema.Client
	entries map[tripleKey]*pooledEntry
}

// NewRegistry creates a new, empty schema client registry. sc is the
// underlying schema.Client (local or remote) used to construct pooled
// instances when a schema identity isn't pooled yet.
func NewRegistry(sc schema.Client) *Registry {
	return &Registry{
		sc:      sc,
		entries: make(map[tripleKey]*pooledEntry),
	}
}

// GetOrCreate returns a schema-bound handle for the schema identity described
// by cfg. If a pooled instance for that identity already exists, its
// refcount is incremented and a handle to that existing instance is
// returned. Otherwise a new instance is constructed via NewSchemaClientBound,
// pooled with refcount 1, and a handle to it is returned. GetOrCreate is safe
// for concurrent use.
func (r *Registry) GetOrCreate(cfg *config.SchemaConfig) *Handle {
	key := tripleKeyFromConfig(cfg)

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok {
		entry = &pooledEntry{
			instance: NewSchemaClientBound(cfg, r.sc),
		}
		r.entries[key] = entry
	}
	entry.refcount++

	return &Handle{
		registry: r,
		key:      key,
		instance: entry.instance,
	}
}

// release decrements the refcount for key and evicts the pooled entry once
// no datastore holds a handle to it anymore. Releasing an unknown triple, or
// one already at zero refcount, is a defensive no-op.
func (r *Registry) release(key tripleKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok || entry.refcount <= 0 {
		return
	}

	entry.refcount--
	if entry.refcount == 0 {
		delete(r.entries, key)
	}
}

// Handle is a datastore's hold on a pooled SchemaClientBoundImpl acquired
// from a Registry: the "schema-bound handle" from the schema caching
// glossary. It behaves as an ordinary SchemaClientBound for lookups, and
// Close releases the registry's hold on the pooled instance.
//
// Handle implements SchemaClientBound but intentionally does not become part
// of that interface itself; Close is only reachable through the concrete
// *Handle type returned by GetOrCreate.
type Handle struct {
	registry *Registry
	key      tripleKey
	instance *SchemaClientBoundImpl

	closeOnce sync.Once
}

// GetSchemaSdcpbPath delegates to the pooled instance.
func (h *Handle) GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return h.instance.GetSchemaSdcpbPath(ctx, path)
}

// GetSchemaElements delegates to the pooled instance.
func (h *Handle) GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error) {
	return h.instance.GetSchemaElements(ctx, p, done)
}

// Close releases this handle's hold on the pooled instance, decrementing the
// registry's refcount for its schema identity and evicting the pooled
// instance once no handle references it anymore. Close is safe to call more
// than once; only the first call has any effect.
func (h *Handle) Close() {
	h.closeOnce.Do(func() {
		h.registry.release(h.key)
	})
}

// Assure Handle implements SchemaClientBound.
var _ SchemaClientBound = (*Handle)(nil)
