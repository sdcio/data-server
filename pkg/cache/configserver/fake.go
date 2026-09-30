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

// FakeLocalConfigReader is an in-memory LocalConfigReader, seedable with test
// fixtures. It stands in for the real config-server local-read transport in
// unit tests, so the config-server-backed cache.Client can be built and fully
// tested well before that transport exists.
type FakeLocalConfigReader struct {
	mu   sync.RWMutex
	docs map[Target]map[string]*Document
}

// NewFakeLocalConfigReader returns an empty FakeLocalConfigReader. Use Seed
// to populate it with fixtures.
func NewFakeLocalConfigReader() *FakeLocalConfigReader {
	return &FakeLocalConfigReader{
		docs: map[Target]map[string]*Document{},
	}
}

// Seed adds/replaces Documents for target, keyed by Document.Name.
func (f *FakeLocalConfigReader) Seed(target Target, docs ...*Document) {
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
func (f *FakeLocalConfigReader) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = map[Target]map[string]*Document{}
}

func (f *FakeLocalConfigReader) Get(ctx context.Context, target Target, name string) (*Document, error) {
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

func (f *FakeLocalConfigReader) List(ctx context.Context, target Target) ([]*Document, error) {
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

var _ LocalConfigReader = (*FakeLocalConfigReader)(nil)
