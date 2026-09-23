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

package configserver

import (
	"context"
	"sort"
	"sync"
)

// FakeLocalConfigClient is an in-memory LocalConfigClient, seedable with
// test fixtures. It stands in for the real config-server local-read/write
// transport in unit tests, so the config-server-backed cache.Client can be
// built and fully tested against realistic read-after-write behavior
// (Modify/Delete mutate the same map Get/List read from) without a live
// config-server.
type FakeLocalConfigClient struct {
	mu   sync.RWMutex
	docs map[Target]map[string]*Document
}

// NewFakeLocalConfigClient returns an empty FakeLocalConfigClient. Use Seed
// to populate it with fixtures.
func NewFakeLocalConfigClient() *FakeLocalConfigClient {
	return &FakeLocalConfigClient{
		docs: map[Target]map[string]*Document{},
	}
}

// Seed adds/replaces Documents for target, keyed by Document.Name.
func (f *FakeLocalConfigClient) Seed(target Target, docs ...*Document) {
	f.mu.Lock()
	defer f.mu.Unlock()

	byName, ok := f.docs[target]
	if !ok {
		byName = map[string]*Document{}
		f.docs[target] = byName
	}
	for _, d := range docs {
		byName[d.Name] = d
	}
}

// Reset removes every seeded Document for every target.
func (f *FakeLocalConfigClient) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = map[Target]map[string]*Document{}
}

func (f *FakeLocalConfigClient) Get(ctx context.Context, target Target, name string) (*Document, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	byName, ok := f.docs[target]
	if !ok {
		return nil, ErrNotFound
	}
	doc, ok := byName[name]
	if !ok {
		return nil, ErrNotFound
	}
	return doc, nil
}

func (f *FakeLocalConfigClient) List(ctx context.Context, target Target) ([]*Document, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	byName := f.docs[target]
	result := make([]*Document, 0, len(byName))
	for _, doc := range byName {
		result = append(result, doc)
	}
	// deterministic ordering for tests/callers that care.
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// Modify is Seed for a single Document, under the LocalConfigWriter name —
// same underlying map, so a Modify is immediately visible to Get/List, the
// real-write behavior this fake exists to let tests exercise.
func (f *FakeLocalConfigClient) Modify(ctx context.Context, target Target, doc *Document) error {
	f.Seed(target, doc)
	return nil
}

// Delete removes name from target's Documents. Deleting a name that isn't
// (or is no longer) present is a no-op success, matching the real
// ConfigSnapshotService.Delete contract (see LocalConfigWriter).
func (f *FakeLocalConfigClient) Delete(ctx context.Context, target Target, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.docs[target], name)
	return nil
}

var _ LocalConfigClient = (*FakeLocalConfigClient)(nil)
