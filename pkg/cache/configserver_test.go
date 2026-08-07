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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/cache/configserver"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
)

const (
	testNamespace = "ns1"
	testCacheName = "target1"
)

func newTestConfigServerCache(t *testing.T) (*ConfigServerCache, *configserver.FakeLocalConfigReader) {
	t.Helper()
	reader := configserver.NewFakeLocalConfigReader()
	return NewConfigServerCache(reader, testNamespace), reader
}

// TestConfigServerCache_InstanceIntentGet_FieldMapping verifies
// InstanceIntentGet calls the seam's Get and wraps the result per the ADR's
// field-mapping table, covering it end to end against the fake.
func TestConfigServerCache_InstanceIntentGet_FieldMapping(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)

	sensitivePaths := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "secret"}}}}
	reader.Seed(configserver.Target{Namespace: testNamespace, Name: testCacheName}, &configserver.Document{
		Name:           "intent1",
		Priority:       10,
		NonRevertive:   true,
		Orphan:         true,
		SensitivePaths: sensitivePaths,
		Config: []*configserver.ConfigBlob{
			{Path: "/interface[name=eth0]/config/mtu", Value: []byte(`9000`)},
		},
	})

	adapter, err := c.InstanceIntentGet(ctx, testCacheName, "intent1")
	if err != nil {
		t.Fatalf("InstanceIntentGet() error = %v", err)
	}
	if got := adapter.GetName(); got != "intent1" {
		t.Errorf("GetName() = %q, want %q", got, "intent1")
	}
	if got := adapter.GetPriority(); got != 10 {
		t.Errorf("GetPriority() = %d, want 10", got)
	}
	if !adapter.GetNonRevertive() {
		t.Error("GetNonRevertive() = false, want true")
	}
	if !adapter.GetOrphan() {
		t.Error("GetOrphan() = false, want true")
	}
}

func TestConfigServerCache_InstanceIntentGet_NotFound(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	_, err := c.InstanceIntentGet(ctx, testCacheName, "missing")
	if !errors.Is(err, configserver.ErrNotFound) {
		t.Fatalf("InstanceIntentGet() error = %v, want ErrNotFound", err)
	}
}

func TestConfigServerCache_InstanceIntentsList(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testCacheName}
	reader.Seed(target,
		&configserver.Document{Name: "intent2"},
		&configserver.Document{Name: "intent1"},
	)

	got, err := c.InstanceIntentsList(ctx, testCacheName)
	if err != nil {
		t.Fatalf("InstanceIntentsList() error = %v", err)
	}
	if diff := cmp.Diff([]string{"intent1", "intent2"}, got); diff != "" {
		t.Errorf("InstanceIntentsList() mismatch (-want +got):\n%s", diff)
	}
}

func TestConfigServerCache_InstanceIntentGetAll(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testCacheName}
	reader.Seed(target,
		&configserver.Document{Name: "intent1", Priority: 1},
		&configserver.Document{Name: "intent2", Priority: 2},
	)

	intentChan := make(chan importer.ImportConfigAdapter)
	errChan := make(chan error, 1)
	go c.InstanceIntentGetAll(ctx, testCacheName, nil, intentChan, errChan)

	var names []string
	for adapter := range intentChan {
		names = append(names, adapter.GetName())
	}
	if err := <-errChan; err != nil {
		t.Fatalf("InstanceIntentGetAll() error = %v", err)
	}
	if diff := cmp.Diff([]string{"intent1", "intent2"}, names); diff != "" {
		t.Errorf("InstanceIntentGetAll() names mismatch (-want +got):\n%s", diff)
	}
}

func TestConfigServerCache_InstanceIntentGetAll_NoDocuments(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	intentChan := make(chan importer.ImportConfigAdapter)
	errChan := make(chan error, 1)
	go c.InstanceIntentGetAll(ctx, testCacheName, nil, intentChan, errChan)

	var names []string
	for adapter := range intentChan {
		names = append(names, adapter.GetName())
	}
	if err := <-errChan; err != nil {
		t.Fatalf("InstanceIntentGetAll() error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("InstanceIntentGetAll() names = %v, want empty", names)
	}
}

// TestConfigServerCache_InstanceIntentGetAll_ContextCancelled verifies
// InstanceIntentGetAll stops sending and returns once ctx is cancelled,
// per the ADR ("closing intentChan/errChan when done or on ctx.Done()"),
// rather than blocking forever on an unbuffered, unread channel.
func TestConfigServerCache_InstanceIntentGetAll_ContextCancelled(t *testing.T) {
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testCacheName}
	reader.Seed(target,
		&configserver.Document{Name: "intent1"},
		&configserver.Document{Name: "intent2"},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	intentChan := make(chan importer.ImportConfigAdapter)
	errChan := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		c.InstanceIntentGetAll(ctx, testCacheName, nil, intentChan, errChan)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("InstanceIntentGetAll did not return after ctx cancellation")
	}

	if _, open := <-intentChan; open {
		t.Error("intentChan should be closed after ctx cancellation")
	}
	if _, open := <-errChan; open {
		t.Error("errChan should be closed after ctx cancellation")
	}
}

func TestConfigServerCache_InstanceIntentExists(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testCacheName}
	reader.Seed(target, &configserver.Document{Name: "intent1"})

	exists, err := c.InstanceIntentExists(ctx, testCacheName, "intent1")
	if err != nil {
		t.Fatalf("InstanceIntentExists() error = %v", err)
	}
	if !exists {
		t.Error("InstanceIntentExists() = false, want true")
	}

	exists, err = c.InstanceIntentExists(ctx, testCacheName, "missing")
	if err != nil {
		t.Fatalf("InstanceIntentExists() error = %v, want nil (not-found maps to false, nil)", err)
	}
	if exists {
		t.Error("InstanceIntentExists() = true, want false")
	}
}

// TestConfigServerCache_WritesAreNoOps verifies InstanceIntentModify and
// InstanceIntentDelete never error, never panic, and never reach the seam
// (the fake has no Modify/Delete methods at all — only Get/List — so any
// attempt to use it that way wouldn't compile in the first place).
func TestConfigServerCache_WritesAreNoOps(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	if err := c.InstanceIntentModify(ctx, testCacheName, &tree_persist.Intent{IntentName: "intent1"}); err != nil {
		t.Errorf("InstanceIntentModify() error = %v, want nil", err)
	}
	if err := c.InstanceIntentDelete(ctx, testCacheName, "intent1", false); err != nil {
		t.Errorf("InstanceIntentDelete() error = %v, want nil", err)
	}

	// A write must not make the intent appear/disappear from the seam's
	// perspective — this backend never touches it either way.
	if _, err := c.InstanceIntentGet(ctx, testCacheName, "intent1"); !errors.Is(err, configserver.ErrNotFound) {
		t.Errorf("InstanceIntentGet() error = %v, want ErrNotFound (write must be a no-op)", err)
	}
}

func TestConfigServerCache_InstanceLifecycle(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	if c.InstanceExists(ctx, testCacheName) {
		t.Error("InstanceExists() = true before InstanceCreate")
	}
	if err := c.InstanceCreate(ctx, testCacheName); err != nil {
		t.Fatalf("InstanceCreate() error = %v", err)
	}
	if !c.InstanceExists(ctx, testCacheName) {
		t.Error("InstanceExists() = false after InstanceCreate")
	}
	if err := c.InstanceCreate(ctx, testCacheName); err == nil {
		t.Error("InstanceCreate() second call: want error, got nil")
	}
	if diff := cmp.Diff([]string{testCacheName}, c.InstancesList(ctx)); diff != "" {
		t.Errorf("InstancesList() mismatch (-want +got):\n%s", diff)
	}
	if err := c.InstanceDelete(ctx, testCacheName); err != nil {
		t.Fatalf("InstanceDelete() error = %v", err)
	}
	if c.InstanceExists(ctx, testCacheName) {
		t.Error("InstanceExists() = true after InstanceDelete")
	}
	if err := c.InstanceDelete(ctx, testCacheName); err == nil {
		t.Error("InstanceDelete() on missing instance: want error, got nil")
	}
}

// TestConfigServerCache_RunningIndependentOfSeam verifies InstanceRunningGet
// / InstanceRunningModify work purely off the in-memory store, entirely
// independent of the LocalConfigReader seam (which never sees "running" at
// all under this backend).
func TestConfigServerCache_RunningIndependentOfSeam(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)

	if err := c.InstanceCreate(ctx, testCacheName); err != nil {
		t.Fatalf("InstanceCreate() error = %v", err)
	}

	want := &tree_persist.Intent{IntentName: "running", Priority: 1}
	if err := c.InstanceRunningModify(ctx, testCacheName, want); err != nil {
		t.Fatalf("InstanceRunningModify() error = %v", err)
	}

	got, err := c.InstanceRunningGet(ctx, testCacheName)
	if err != nil {
		t.Fatalf("InstanceRunningGet() error = %v", err)
	}
	if got != want {
		t.Errorf("InstanceRunningGet() = %v, want %v", got, want)
	}

	// The seam was never seeded with anything and never asked for
	// "running" — List/Get must still be untouched (fake has no docs at
	// all for this target).
	docs, err := reader.List(ctx, configserver.Target{Namespace: testNamespace, Name: testCacheName})
	if err != nil {
		t.Fatalf("reader.List() error = %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("reader.List() = %v, want empty (running never touches the seam)", docs)
	}
}

func TestConfigServerCache_InstanceRunningGet_NotYetModified(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	if err := c.InstanceCreate(ctx, testCacheName); err != nil {
		t.Fatalf("InstanceCreate() error = %v", err)
	}

	_, err := c.InstanceRunningGet(ctx, testCacheName)
	if !errors.Is(err, ErrRunningNotFound) {
		t.Fatalf("InstanceRunningGet() error = %v, want ErrRunningNotFound", err)
	}
}

func TestConfigServerCache_InstanceRunningGet_UnknownInstance(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	_, err := c.InstanceRunningGet(ctx, "unknown")
	if err == nil {
		t.Fatal("InstanceRunningGet() on unknown instance: want error, got nil")
	}
}

func TestConfigServerCache_ImplementsClient(t *testing.T) {
	var _ Client = NewConfigServerCache(configserver.NewFakeLocalConfigReader(), testNamespace)
}
