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

package netconf

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree"
	xmlimporter "github.com/sdcio/data-server/pkg/tree/importer/xml"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/consts"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// fakeXMLImporterTarget implements GetXMLImporter, returning a fixed
// (possibly nil) importer or a fixed error for every call.
type fakeXMLImporterTarget struct {
	imp importer.ImportConfigAdapter
	err error
}

func (f *fakeXMLImporterTarget) GetImportAdapter(_ context.Context, _ *sdcpb.GetDataRequest) (importer.ImportConfigAdapter, error) {
	return f.imp, f.err
}

// fakeNetconfRunningStore implements types.RunningStore, recording every
// MarkSynced call and returning a configurable error from ApplyToRunning.
type fakeNetconfRunningStore struct {
	applyErr error

	mu              sync.Mutex
	synced          map[string]bool
	lastImporterNil bool
	lastPathCount   int
}

func newFakeNetconfRunningStore(applyErr error) *fakeNetconfRunningStore {
	return &fakeNetconfRunningStore{applyErr: applyErr, synced: make(map[string]bool)}
}

func (f *fakeNetconfRunningStore) ApplyToRunning(_ context.Context, paths []*sdcpb.Path, imp importer.ImportConfigAdapter) error {
	f.mu.Lock()
	f.lastImporterNil = imp == nil
	f.lastPathCount = len(paths)
	f.mu.Unlock()
	return f.applyErr
}

func (f *fakeNetconfRunningStore) applySnapshot() (importerNil bool, pathCount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastImporterNil, f.lastPathCount
}

func (f *fakeNetconfRunningStore) NewEmptyTree(_ context.Context) (*tree.RootEntry, error) {
	return nil, nil
}

func (f *fakeNetconfRunningStore) MarkSynced(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.synced[name] = true
}

func (f *fakeNetconfRunningStore) isSynced(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.synced[name]
}

// TestNetconfSyncImpl_InternalSync_MarksSyncedOnlyOnSuccess verifies that
// internalSync calls runningStore.MarkSynced for its own sync name after a
// successful ApplyToRunning, and never when ApplyToRunning fails.
func TestNetconfSyncImpl_InternalSync_MarksSyncedOnlyOnSuccess(t *testing.T) {
	const syncName = "nc-sync"
	c := &config.SyncProtocol{Name: syncName}

	t.Run("ApplyToRunning fails: not marked Synced", func(t *testing.T) {
		store := newFakeNetconfRunningStore(errors.New("apply failed"))
		s, err := NewNetconfSyncImpl(context.Background(), "target", &fakeXMLImporterTarget{}, c, store)
		if err != nil {
			t.Fatalf("NewNetconfSyncImpl: %v", err)
		}

		if err := s.internalSync(&sdcpb.GetDataRequest{}); err == nil {
			t.Fatal("internalSync() error = nil, want non-nil")
		}
		if store.isSynced(syncName) {
			t.Error("runningStore reports Synced after a failed ApplyToRunning, want not-Synced")
		}
	})

	t.Run("ApplyToRunning succeeds: marked Synced", func(t *testing.T) {
		store := newFakeNetconfRunningStore(nil)
		s, err := NewNetconfSyncImpl(context.Background(), "target", &fakeXMLImporterTarget{}, c, store)
		if err != nil {
			t.Fatalf("NewNetconfSyncImpl: %v", err)
		}

		if err := s.internalSync(&sdcpb.GetDataRequest{}); err != nil {
			t.Fatalf("internalSync() error = %v, want nil", err)
		}
		if !store.isSynced(syncName) {
			t.Errorf("runningStore.MarkSynced(%q) was not called after a successful ApplyToRunning", syncName)
		}
	})

	t.Run("fetch fails: not marked Synced", func(t *testing.T) {
		store := newFakeNetconfRunningStore(nil)
		target := &fakeXMLImporterTarget{err: errors.New("get failed")}
		s, err := NewNetconfSyncImpl(context.Background(), "target", target, c, store)
		if err != nil {
			t.Fatalf("NewNetconfSyncImpl: %v", err)
		}

		if err := s.internalSync(&sdcpb.GetDataRequest{}); err == nil {
			t.Fatal("internalSync() error = nil, want non-nil")
		}
		if store.isSynced(syncName) {
			t.Error("runningStore reports Synced after a failed fetch, want not-Synced")
		}
	})

	t.Run("nil importer: scoped refresh and marked Synced", func(t *testing.T) {
		store := newFakeNetconfRunningStore(nil)
		cWithPaths := &config.SyncProtocol{
			Name:  syncName,
			Paths: []string{"/interface"},
		}
		s, err := NewNetconfSyncImpl(context.Background(), "target", &fakeXMLImporterTarget{}, cWithPaths, store)
		if err != nil {
			t.Fatalf("NewNetconfSyncImpl: %v", err)
		}

		if err := s.internalSync(&sdcpb.GetDataRequest{}); err != nil {
			t.Fatalf("internalSync() error = %v, want nil", err)
		}
		importerNil, pathCount := store.applySnapshot()
		if !importerNil {
			t.Error("ApplyToRunning importer: want nil for empty snapshot")
		}
		if pathCount != len(cWithPaths.Paths) {
			t.Errorf("ApplyToRunning paths: got %d, want %d", pathCount, len(cWithPaths.Paths))
		}
		if !store.isSynced(syncName) {
			t.Errorf("runningStore.MarkSynced(%q) was not called after empty sync cycle", syncName)
		}
	})

	t.Run("empty XML importer: nil importer at apply seam and marked Synced", func(t *testing.T) {
		store := newFakeNetconfRunningStore(nil)
		doc := etree.NewDocument()
		root := doc.CreateElement("config")
		emptyImp := xmlimporter.NewXmlTreeImporter(root, consts.RunningIntentName, consts.RunningValuesPrio, false)
		target := &fakeXMLImporterTarget{imp: emptyImp}
		cWithPaths := &config.SyncProtocol{
			Name:  syncName,
			Paths: []string{"/interface"},
		}
		s, err := NewNetconfSyncImpl(context.Background(), "target", target, cWithPaths, store)
		if err != nil {
			t.Fatalf("NewNetconfSyncImpl: %v", err)
		}

		if err := s.internalSync(&sdcpb.GetDataRequest{}); err != nil {
			t.Fatalf("internalSync() error = %v, want nil", err)
		}
		importerNil, _ := store.applySnapshot()
		if !importerNil {
			t.Error("ApplyToRunning importer: want nil when target returned empty XML snapshot")
		}
		if !store.isSynced(syncName) {
			t.Errorf("runningStore.MarkSynced(%q) was not called after empty XML sync cycle", syncName)
		}
	})
}
