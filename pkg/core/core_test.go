package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	schemaConfig "github.com/sdcio/schema-server/pkg/config"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.New("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.SchemaStore = &schemaConfig.SchemaStoreConfig{
		Type: schemaConfig.StoreTypeMemory,
		Schemas: []*schemaConfig.SchemaConfig{{
			Name: "", Vendor: "v", Version: "1",
			Files: []string{"../../tests/schema/sdcio_model.yang"}, Directories: []string{"../../tests/schema"},
		}},
	}
	cfg.Cache = &config.CacheConfig{Type: "local", StoreType: "badgerdbsharded", Dir: t.TempDir()}
	cfg.Datastores = []*config.DatastoreConfig{{
		Name:   "ds1",
		Schema: &config.SchemaConfig{Name: "", Vendor: "v", Version: "1"},
		SBI:    &config.SBI{Type: "noop"},
		Sync:   &config.Sync{},
	}}
	for _, ds := range cfg.Datastores {
		if err := ds.ValidateSetDefaults(); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func startCore(t *testing.T) *Core {
	t.Helper()
	ctx := context.Background()
	c, err := New(ctx, newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx) })
	go func() { _ = c.Start(ctx) }()

	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := c.WaitReady(wctx); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGetDataStoreInProcess(t *testing.T) {
	c := startCore(t)

	info, err := c.GetDataStore(context.Background(), "ds1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "ds1" || info.Spec.Name != "ds1" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Spec.Target.Type != api.TargetType("noop") {
		t.Fatalf("target type = %q", info.Spec.Target.Type)
	}
	if info.Spec.Schema != (api.SchemaRef{Vendor: "v", Version: "1"}) {
		t.Fatalf("schema = %+v", info.Spec.Schema)
	}
}

func TestGetDataStoreErrors(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()

	if _, err := c.GetDataStore(ctx, ""); !errors.Is(err, api.ErrInvalidArgument) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := c.GetDataStore(ctx, "nope"); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	c := startCore(t)
	ctx := context.Background()
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestWatchDeviationsClosesOnCancel(t *testing.T) {
	c := startCore(t)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.WatchDeviations(ctx, "ds1")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("unexpected event")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed")
	}
	if _, err := c.WatchDeviations(context.Background(), "nope"); !errors.Is(err, api.ErrUnknownDatastore) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestWaitReadyHonoursContext(t *testing.T) {
	c, err := New(context.Background(), newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.WaitReady(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestStartAfterStopFails(t *testing.T) {
	c, err := New(context.Background(), newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Stop(context.Background())
	if err := c.Start(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestStartFailsWhenInitialDatastoreFails(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Datastores[0].Schema.Name = "missing"
	c, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if err := c.WaitReady(context.Background()); err == nil {
		t.Fatal("WaitReady should report the failure")
	}
}
