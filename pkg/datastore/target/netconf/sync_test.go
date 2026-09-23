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

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
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

	mu     sync.Mutex
	synced map[string]bool
}

func newFakeNetconfRunningStore(applyErr error) *fakeNetconfRunningStore {
	return &fakeNetconfRunningStore{applyErr: applyErr, synced: make(map[string]bool)}
}

func (f *fakeNetconfRunningStore) ApplyToRunning(_ context.Context, _ []*sdcpb.Path, _ importer.ImportConfigAdapter) error {
	return f.applyErr
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
}
