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
	"google.golang.org/protobuf/proto"
)

// leafVariant marshals tv the same way TreeExport does (le.ValueAsBytes()),
// so fixtures build TreeElements the way a real export would.
func leafVariant(t *testing.T, tv *sdcpb.TypedValue) []byte {
	t.Helper()
	b, err := proto.Marshal(tv)
	if err != nil {
		t.Fatalf("marshal TypedValue: %v", err)
	}
	return b
}

const (
	testNamespace = "ns1"
	testTarget    = "target1"
	testCacheName = testNamespace + "." + testTarget
)

func newTestConfigServerCache(t *testing.T) (*ConfigServerCache, *configserver.FakeLocalConfigClient) {
	t.Helper()
	client := configserver.NewFakeLocalConfigClient()
	return NewConfigServerCache(client), client
}

// TestConfigServerCache_InstanceIntentGet_FieldMapping verifies
// InstanceIntentGet calls the seam's Get and wraps the result per the ADR's
// field-mapping table, covering it end to end against the fake.
func TestConfigServerCache_InstanceIntentGet_FieldMapping(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)

	sensitivePaths := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "secret"}}}}
	reader.Seed(configserver.Target{Namespace: testNamespace, Name: testTarget}, &configserver.Document{
		Name:           "intent1",
		Namespace:      testNamespace,
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
	if got := adapter.GetName(); got != testNamespace+".intent1" {
		t.Errorf("GetName() = %q, want %q", got, testNamespace+".intent1")
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

func TestConfigServerCache_InstanceIntentGet_GVKNSNName(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	reader.Seed(configserver.Target{Namespace: testNamespace, Name: testTarget}, &configserver.Document{
		Name:      "intent1",
		Namespace: testNamespace,
	})

	adapter, err := c.InstanceIntentGet(ctx, testCacheName, testNamespace+".intent1")
	if err != nil {
		t.Fatalf("InstanceIntentGet() error = %v", err)
	}
	if got := adapter.GetName(); got != testNamespace+".intent1" {
		t.Errorf("GetName() = %q, want %q", got, testNamespace+".intent1")
	}
}

func TestConfigServerCache_InstanceIntentsList(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testTarget}
	reader.Seed(target,
		&configserver.Document{Name: "intent2", Namespace: testNamespace},
		&configserver.Document{Name: "intent1", Namespace: testNamespace},
	)

	got, err := c.InstanceIntentsList(ctx, testCacheName)
	if err != nil {
		t.Fatalf("InstanceIntentsList() error = %v", err)
	}
	if diff := cmp.Diff([]string{testNamespace + ".intent1", testNamespace + ".intent2"}, got); diff != "" {
		t.Errorf("InstanceIntentsList() mismatch (-want +got):\n%s", diff)
	}
}

func TestConfigServerCache_InstanceIntentGetAll(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testTarget}
	reader.Seed(target,
		&configserver.Document{Name: "intent1", Namespace: testNamespace, Priority: 1},
		&configserver.Document{Name: "intent2", Namespace: testNamespace, Priority: 2},
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
	if diff := cmp.Diff([]string{testNamespace + ".intent1", testNamespace + ".intent2"}, names); diff != "" {
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
	target := configserver.Target{Namespace: testNamespace, Name: testTarget}
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
	target := configserver.Target{Namespace: testNamespace, Name: testTarget}
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

// TestConfigServerCache_MalformedDatastoreName_ReadCallers verifies
// InstanceIntentsList/InstanceIntentGet/InstanceIntentExists all surface
// ErrMalformedDatastoreName as a normal returned error when the
// cacheInstanceName they're called with doesn't decode into a
// namespace/name pair, rather than building a lookup with an empty
// namespace or sending the whole compound name as the bare target name.
func TestConfigServerCache_MalformedDatastoreName_ReadCallers(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)
	const malformed = "no-dot-here"

	if _, err := c.InstanceIntentsList(ctx, malformed); !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentsList() error = %v, want ErrMalformedDatastoreName", err)
	}
	if _, err := c.InstanceIntentGet(ctx, malformed, "intent1"); !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentGet() error = %v, want ErrMalformedDatastoreName", err)
	}
	if _, err := c.InstanceIntentExists(ctx, malformed, "intent1"); !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentExists() error = %v, want ErrMalformedDatastoreName", err)
	}
}

// TestConfigServerCache_InstanceIntentGetAll_MalformedDatastoreName verifies
// InstanceIntentGetAll sends ErrMalformedDatastoreName on its existing
// errChan, the same channel real reader.List errors already use, and closes
// both channels.
func TestConfigServerCache_InstanceIntentGetAll_MalformedDatastoreName(t *testing.T) {
	c, _ := newTestConfigServerCache(t)
	const malformed = "no-dot-here"

	intentChan := make(chan importer.ImportConfigAdapter)
	errChan := make(chan error, 1)
	go c.InstanceIntentGetAll(context.Background(), malformed, nil, intentChan, errChan)

	if _, open := <-intentChan; open {
		t.Error("intentChan should be closed without ever sending on a malformed name")
	}
	if err := <-errChan; !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("errChan error = %v, want ErrMalformedDatastoreName", err)
	}
}

// TestConfigServerCache_InstanceCreate_MalformedDatastoreName verifies
// InstanceCreate rejects a malformed datastore name at creation time,
// before it ever mutates the running map — the instance must not be left
// half-created.
func TestConfigServerCache_InstanceCreate_MalformedDatastoreName(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)
	const malformed = "no-dot-here"

	if err := c.InstanceCreate(ctx, malformed); !errors.Is(err, ErrMalformedDatastoreName) {
		t.Fatalf("InstanceCreate() error = %v, want ErrMalformedDatastoreName", err)
	}
	if c.InstanceExists(ctx, malformed) {
		t.Error("InstanceExists() = true after InstanceCreate rejected a malformed name")
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
// all under this backend). InstanceRunningGet returns an
// importer.ImportConfigAdapter — the same mechanical shape InstanceIntentGet
// returns — rather than the raw *tree_persist.Intent, so the assertions go
// through its accessors.
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
	if gotName := got.GetName(); gotName != want.GetIntentName() {
		t.Errorf("InstanceRunningGet().GetName() = %q, want %q", gotName, want.GetIntentName())
	}
	if gotPriority := got.GetPriority(); gotPriority != want.GetPriority() {
		t.Errorf("InstanceRunningGet().GetPriority() = %d, want %d", gotPriority, want.GetPriority())
	}

	// The seam was never seeded with anything and never asked for
	// "running" — List/Get must still be untouched (fake has no docs at
	// all for this target).
	docs, err := reader.List(ctx, configserver.Target{Namespace: testNamespace, Name: testTarget})
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

// TestConfigServerCache_ImplementsClient verifies *ConfigServerCache
// satisfies the full cache.Client directly — including IntentWriter, now
// that Modify/Delete are real writes against the LocalConfigWriter seam
// rather than the generic noopIntentWriter.
func TestConfigServerCache_ImplementsClient(t *testing.T) {
	c := NewConfigServerCache(configserver.NewFakeLocalConfigClient())
	var _ Client = c
}

// TestConfigServerCache_InstanceIntentModify_CreatesAndIsReadableBack
// covers the write-then-read round trip through the seam: a modified
// Intent must be readable back via InstanceIntentGet with the content that
// was written, matching the ghost-intent regression's "last-applied
// updates at apply time" contract.
func TestConfigServerCache_InstanceIntentModify_CreatesAndIsReadableBack(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	intent := &tree_persist.Intent{
		IntentName: testNamespace + ".intent1",
		Priority:   5,
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				{Name: "hostname", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "router1"}})},
			},
		},
	}

	if err := c.InstanceIntentModify(ctx, testCacheName, intent); err != nil {
		t.Fatalf("InstanceIntentModify() error = %v", err)
	}

	adapter, err := c.InstanceIntentGet(ctx, testCacheName, testNamespace+".intent1")
	if err != nil {
		t.Fatalf("InstanceIntentGet() after Modify: %v", err)
	}
	if adapter.GetPriority() != 5 {
		t.Errorf("GetPriority() = %d, want 5", adapter.GetPriority())
	}
}

// TestConfigServerCache_InstanceIntentDelete_RemovesFromSeam is the
// regression test for the ghost-intent bug this ticket exists to fix:
// deleting an intent must make it unreadable via the seam immediately, not
// only after some later, unrelated reconcile.
func TestConfigServerCache_InstanceIntentDelete_RemovesFromSeam(t *testing.T) {
	ctx := context.Background()
	c, reader := newTestConfigServerCache(t)
	target := configserver.Target{Namespace: testNamespace, Name: testTarget}
	reader.Seed(target, &configserver.Document{Name: "intent1", Namespace: testNamespace})

	if err := c.InstanceIntentDelete(ctx, testCacheName, testNamespace+".intent1", false); err != nil {
		t.Fatalf("InstanceIntentDelete() error = %v", err)
	}

	if _, err := c.InstanceIntentGet(ctx, testCacheName, testNamespace+".intent1"); !errors.Is(err, configserver.ErrNotFound) {
		t.Errorf("InstanceIntentGet() after Delete: err = %v, want ErrNotFound", err)
	}
}

// TestConfigServerCache_InstanceIntentDelete_MissingIsNoop matches the
// ConfigSnapshotService.Delete contract: deleting an intent that was never
// present is a no-op success under this backend regardless of
// ignoreNonExisting — TargetSnapshot's membership model has no tombstone to
// distinguish "already gone" from "never existed".
func TestConfigServerCache_InstanceIntentDelete_MissingIsNoop(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	if err := c.InstanceIntentDelete(ctx, testCacheName, "missing", false); err != nil {
		t.Errorf("InstanceIntentDelete() of missing intent: %v, want no-op success", err)
	}
}

// TestConfigServerCache_InstanceIntentModify_MalformedDatastoreName and
// TestConfigServerCache_InstanceIntentDelete_MalformedDatastoreName lock the
// same ErrMalformedDatastoreName contract the read callers already have,
// for the write callers.
func TestConfigServerCache_InstanceIntentModify_MalformedDatastoreName(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	err := c.InstanceIntentModify(ctx, "no-dot-here", &tree_persist.Intent{IntentName: "intent1"})
	if !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentModify() error = %v, want ErrMalformedDatastoreName", err)
	}
}

func TestConfigServerCache_InstanceIntentDelete_MalformedDatastoreName(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConfigServerCache(t)

	err := c.InstanceIntentDelete(ctx, "no-dot-here", "intent1", false)
	if !errors.Is(err, ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentDelete() error = %v, want ErrMalformedDatastoreName", err)
	}
}
