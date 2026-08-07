package cache

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	cacheconfig "github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/testing/protocmp"
)

func newTestLocalCache(t *testing.T) *LocalCache {
	t.Helper()
	c, err := NewLocalCache(&cacheconfig.CacheConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalCache: %v", err)
	}
	lc, ok := c.(*LocalCache)
	if !ok {
		t.Fatalf("NewLocalCache did not return *LocalCache")
	}
	return lc
}

// TestLocalCache_RunningRoundTrip verifies InstanceRunningModify/InstanceRunningGet
// are thin passthroughs to the same disk-backed store InstanceIntentGet/Modify use,
// so the local backend's on-disk behavior for "running" is unchanged by the split.
func TestLocalCache_RunningRoundTrip(t *testing.T) {
	ctx := context.Background()
	lc := newTestLocalCache(t)

	const cacheName = "ds1"
	if err := lc.InstanceCreate(ctx, cacheName); err != nil {
		t.Fatalf("InstanceCreate: %v", err)
	}

	want := &tree_persist.Intent{
		IntentName: consts.RunningIntentName,
		Priority:   consts.RunningValuesPrio,
		Root: &tree_persist.TreeElement{
			Name: "root",
		},
	}

	if err := lc.InstanceRunningModify(ctx, cacheName, want); err != nil {
		t.Fatalf("InstanceRunningModify: %v", err)
	}

	got, err := lc.InstanceRunningGet(ctx, cacheName)
	if err != nil {
		t.Fatalf("InstanceRunningGet: %v", err)
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("InstanceRunningGet mismatch (-want +got):\n%s", diff)
	}
}

// TestLocalCache_RunningExcludedFromIntentGetAll verifies that once callers ask
// for real intents via InstanceIntentGetAll with running explicitly excluded (the
// pattern every real call site now uses), "running" never shows up, even though
// LocalCache still persists it in the same underlying disk-backed store.
func TestLocalCache_RunningExcludedFromIntentGetAll(t *testing.T) {
	ctx := context.Background()
	lc := newTestLocalCache(t)

	const cacheName = "ds1"
	if err := lc.InstanceCreate(ctx, cacheName); err != nil {
		t.Fatalf("InstanceCreate: %v", err)
	}

	if err := lc.InstanceRunningModify(ctx, cacheName, &tree_persist.Intent{
		IntentName: consts.RunningIntentName,
		Priority:   consts.RunningValuesPrio,
	}); err != nil {
		t.Fatalf("InstanceRunningModify: %v", err)
	}

	realIntent := &tree_persist.Intent{
		IntentName: "intent1",
		Priority:   10,
	}
	if err := lc.InstanceIntentModify(ctx, cacheName, realIntent); err != nil {
		t.Fatalf("InstanceIntentModify: %v", err)
	}

	intentChan := make(chan importer.ImportConfigAdapter)
	errChan := make(chan error, 1)
	go lc.InstanceIntentGetAll(ctx, cacheName, []string{consts.RunningIntentName}, intentChan, errChan)

	var names []string
	for intent := range intentChan {
		names = append(names, intent.GetName())
	}
	if err := <-errChan; err != nil {
		t.Fatalf("InstanceIntentGetAll: %v", err)
	}

	if diff := cmp.Diff([]string{"intent1"}, names); diff != "" {
		t.Errorf("InstanceIntentGetAll names mismatch (-want +got):\n%s", diff)
	}
}
