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

// *LocalCache satisfies Client and each capability.
var (
	_ Client = (*LocalCache)(nil)

	_ IntentReader      = (*LocalCache)(nil)
	_ IntentWriter      = (*LocalCache)(nil)
	_ RunningStore      = (*LocalCache)(nil)
	_ InstanceLifecycle = (*LocalCache)(nil)
)

// composedClient is Client as the four capabilities, so a value assembled
// from those parts still satisfies Client.
type composedClient interface {
	IntentReader
	IntentWriter
	RunningStore
	InstanceLifecycle
}

var _ Client = composedClient(nil)
var _ composedClient = Client(nil)

// *CacheClientBoundImpl satisfies CacheClientBound and each bound capability.
var (
	_ CacheClientBound = (*CacheClientBoundImpl)(nil)

	_ BoundIntentReader      = (*CacheClientBoundImpl)(nil)
	_ BoundIntentWriter      = (*CacheClientBoundImpl)(nil)
	_ BoundRunningStore      = (*CacheClientBoundImpl)(nil)
	_ BoundInstanceLifecycle = (*CacheClientBoundImpl)(nil)
)

type composedCacheClientBound interface {
	BoundIntentReader
	BoundIntentWriter
	BoundRunningStore
	BoundInstanceLifecycle
}

var _ CacheClientBound = composedCacheClientBound(nil)
var _ composedCacheClientBound = CacheClientBound(nil)
