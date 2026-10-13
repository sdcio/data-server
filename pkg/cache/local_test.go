package cache

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	cacheconfig "github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/sdc-protos/tree_persist"
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

// TestLocalCache_InstanceIntentGetRoundTrip: InstanceIntentModify then
// InstanceIntentGet returns the stored intent name and priority.
func TestLocalCache_InstanceIntentGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	lc := newTestLocalCache(t)

	const cacheName = "ds1"
	if err := lc.InstanceCreate(ctx, cacheName); err != nil {
		t.Fatalf("InstanceCreate: %v", err)
	}

	want := &tree_persist.Intent{
		IntentName: "intent1",
		Priority:   10,
		Root: &tree_persist.TreeElement{
			Name: "root",
		},
	}

	if err := lc.InstanceIntentModify(ctx, cacheName, want); err != nil {
		t.Fatalf("InstanceIntentModify: %v", err)
	}

	got, err := lc.InstanceIntentGet(ctx, cacheName, "intent1")
	if err != nil {
		t.Fatalf("InstanceIntentGet: %v", err)
	}

	if gotName := got.GetName(); gotName != want.GetIntentName() {
		t.Errorf("InstanceIntentGet().GetName() = %q, want %q", gotName, want.GetIntentName())
	}
	if gotPriority := got.GetPriority(); gotPriority != want.GetPriority() {
		t.Errorf("InstanceIntentGet().GetPriority() = %d, want %d", gotPriority, want.GetPriority())
	}
}

// TestLocalCache_RunningRoundTrip: InstanceRunningModify then InstanceRunningGet
// returns the running intent through ImportConfigAdapter.
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

	if gotName := got.GetName(); gotName != consts.RunningIntentName {
		t.Errorf("InstanceRunningGet().GetName() = %q, want %q", gotName, consts.RunningIntentName)
	}
	if gotPriority := got.GetPriority(); gotPriority != consts.RunningValuesPrio {
		t.Errorf("InstanceRunningGet().GetPriority() = %d, want %d", gotPriority, consts.RunningValuesPrio)
	}
}

// TestLocalCache_RunningExcludedFromIntentGetAll: excluding the running intent
// name keeps it out of InstanceIntentGetAll, even though it shares the disk store.
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

	intentChan := make(chan importer.IntentAdapter)
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
