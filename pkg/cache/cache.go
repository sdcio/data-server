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

// IntentReader reads real Intents. Every cache backend implements it.
type IntentReader interface {
	InstanceIntentsList(ctx context.Context, cacheInstanceName string) ([]string, error)
	InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.IntentAdapter, error)
	InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error)
	InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChan chan<- importer.IntentAdapter, errChan chan<- error)
}

// IntentWriter persists real Intents. Every current backend (LocalCache,
// ConfigServerCache) implements this directly. A future backend that
// genuinely cannot own real-Intent writes would need an explicit no-op
// composition at Server.createCacheClient so the type system makes that
// lack of ownership visible.
type IntentWriter interface {
	InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
	InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error
}

// RunningStore reads and writes "running", the synced device-state intent.
// It is independent of Cache.Type: LocalCache keeps it on disk; other
// backends may keep it in memory.
type RunningStore interface {
	InstanceRunningGet(ctx context.Context, cacheName string) (importer.ImportConfigAdapter, error)
	InstanceRunningModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
}

// InstanceLifecycle creates, closes, and enumerates cache instances.
// On a config-server backend this covers only the in-memory running store,
// not whether real Intent data exists for a target.
type InstanceLifecycle interface {
	InstanceCreate(ctx context.Context, cacheInstanceName string) error
	InstanceDelete(ctx context.Context, cacheInstanceName string) error
	InstanceClose(ctx context.Context, cacheInstanceName string) error
	InstanceExists(ctx context.Context, cacheInstanceName string) bool
	InstancesList(ctx context.Context) []string
}

// Client is a full cache backend: intent read and write, running, and
// instance lifecycle.
type Client interface {
	IntentReader
	IntentWriter
	RunningStore
	InstanceLifecycle
}
