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

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/sdc-protos/tree_persist"
)

// IntentReader reads real Intents. Every backend implements this directly —
// it's the one capability every Cache.Type genuinely has data for.
type IntentReader interface {
	InstanceIntentsList(ctx context.Context, cacheInstanceName string) ([]string, error)
	InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.ImportConfigAdapter, error)
	InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error)
	InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error)
}

// IntentWriter persists real Intents. Backends that don't own real-Intent
// writes (e.g. config-server, where kube-api owns them) compose a no-op
// implementation instead of implementing this directly, so the type system
// makes that lack of ownership visible.
type IntentWriter interface {
	InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
	InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error
}

// RunningStore reads/writes "running": split out of the generic Intent
// surface, independent of Cache.Type. LocalCache keeps its existing
// disk-backed store for it (unchanged on-disk behavior); other backends may
// back it however they see fit (e.g. purely in-memory), since it's never
// config-server's data to begin with.
type RunningStore interface {
	InstanceRunningGet(ctx context.Context, cacheName string) (importer.ImportConfigAdapter, error)
	InstanceRunningModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
}

// InstanceLifecycle creates/destroys/enumerates cache instances. Under
// ConfigServerCache this only ever governs its in-memory running store — it
// has no relationship to whether real-Intent data exists for a target under
// that backend, unlike LocalCache where it governs the one shared disk store
// underlying everything.
type InstanceLifecycle interface {
	InstanceCreate(ctx context.Context, cacheInstanceName string) error
	InstanceDelete(ctx context.Context, cacheInstanceName string) error
	InstanceClose(ctx context.Context, cacheInstanceName string) error
	InstanceExists(ctx context.Context, cacheInstanceName string) bool
	InstancesList(ctx context.Context) []string
}

type Client interface {
	IntentReader
	IntentWriter
	RunningStore
	InstanceLifecycle
}
