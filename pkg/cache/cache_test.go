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

// Compile-time assertions that the capability interfaces compose back into
// Client with no change to its method surface, and that *LocalCache keeps
// satisfying Client (and, by extension, all four capabilities) with zero
// code changes.
var (
	_ Client = (*LocalCache)(nil)

	_ IntentReader      = (*LocalCache)(nil)
	_ IntentWriter      = (*LocalCache)(nil)
	_ RunningStore      = (*LocalCache)(nil)
	_ InstanceLifecycle = (*LocalCache)(nil)
)

// composedClient documents that Client is exactly the sum of the four
// capabilities — nothing more, nothing less — from the perspective of any
// caller assembling one from the parts.
type composedClient interface {
	IntentReader
	IntentWriter
	RunningStore
	InstanceLifecycle
}

var _ Client = composedClient(nil)
var _ composedClient = Client(nil)

// Same compile-time shape check for the bound side: CacheClientBoundImpl
// already implements every method needed, so it should keep satisfying
// CacheClientBound and each bound capability with zero code changes.
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
