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

package schemaClient

import (
	"context"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"

	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/pkg/config"
)

func testSchemaConfig(name, vendor, version string) *config.SchemaConfig {
	return &config.SchemaConfig{
		Name:    name,
		Vendor:  vendor,
		Version: version,
	}
}

func testPath() *sdcpb.Path {
	return &sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			{Name: "interface"},
		},
	}
}

func TestRegistry_GetOrCreate_SameTriple_SharesPooledInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)
	// The underlying schema client should only ever be asked for this path
	// once: both handles share the same pooled instance and thus the same
	// path-keyed lookup cache.
	sc.EXPECT().
		GetSchema(gomock.Any(), gomock.Any()).
		Return(&sdcpb.GetSchemaResponse{}, nil).
		Times(1)

	registry := NewRegistry(sc)
	cfg := testSchemaConfig("os", "vendor", "1.0.0")

	h1 := registry.GetOrCreate(cfg)
	h2 := registry.GetOrCreate(cfg)

	if h1.instance != h2.instance {
		t.Fatalf("expected GetOrCreate with the same triple to return handles backed by the same pooled instance")
	}

	ctx := context.Background()
	if _, err := h1.GetSchemaSdcpbPath(ctx, testPath()); err != nil {
		t.Fatalf("unexpected error from h1.GetSchemaSdcpbPath: %v", err)
	}
	// Retrieving the same path through h2 must hit the shared cache instead
	// of calling the underlying schema client again (enforced by Times(1)
	// above).
	if _, err := h2.GetSchemaSdcpbPath(ctx, testPath()); err != nil {
		t.Fatalf("unexpected error from h2.GetSchemaSdcpbPath: %v", err)
	}
}

func TestRegistry_GetOrCreate_DifferentTriples_DistinctPooledInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)

	registry := NewRegistry(sc)

	h1 := registry.GetOrCreate(testSchemaConfig("os-a", "vendor-a", "1.0.0"))
	h2 := registry.GetOrCreate(testSchemaConfig("os-b", "vendor-b", "2.0.0"))

	if h1.instance == h2.instance {
		t.Fatalf("expected GetOrCreate with different triples to return handles backed by distinct pooled instances")
	}
}

func TestRegistry_Close_KeepsPooledInstanceAliveUntilLastHandleCloses(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)

	registry := NewRegistry(sc)
	cfg := testSchemaConfig("os", "vendor", "1.0.0")

	h1 := registry.GetOrCreate(cfg)
	h2 := registry.GetOrCreate(cfg)

	// Closing one of two handles sharing a triple must keep the pooled
	// instance alive (refcount > 0): a subsequent GetOrCreate for that
	// triple must still return the same instance, not a fresh one.
	h1.Close()

	h3 := registry.GetOrCreate(cfg)
	if h3.instance != h2.instance {
		t.Fatalf("expected the pooled instance to survive while another handle still references it")
	}

	// Closing every remaining handle must evict the pooled entry: a
	// subsequent GetOrCreate constructs a fresh instance.
	h2.Close()
	h3.Close()

	h4 := registry.GetOrCreate(cfg)
	if h4.instance == h2.instance {
		t.Fatalf("expected a fresh pooled instance once every handle to the previous one was closed")
	}

	if got := len(registry.entries); got != 1 {
		t.Fatalf("expected exactly one pooled entry to remain registered, got %d", got)
	}
}

func TestHandle_Close_IsIdempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)

	registry := NewRegistry(sc)
	cfg := testSchemaConfig("os", "vendor", "1.0.0")

	h := registry.GetOrCreate(cfg)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("closing a handle twice must not panic, got: %v", r)
		}
	}()
	h.Close()
	h.Close()

	if got := len(registry.entries); got != 0 {
		t.Fatalf("expected the pooled entry to be evicted after its only handle closed, got %d entries", got)
	}
}

func TestRegistry_Release_UnknownTriple_IsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)

	registry := NewRegistry(sc)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("releasing an unregistered triple must not panic, got: %v", r)
		}
	}()
	registry.release(tripleKey{Name: "unknown", Vendor: "unknown", Version: "unknown"})
}

func TestRegistry_GetOrCreate_ConstructsFreshInstanceOnMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	sc := mockschema.NewMockClient(ctrl)

	registry := NewRegistry(sc)
	cfg := testSchemaConfig("os", "vendor", "1.0.0")

	h := registry.GetOrCreate(cfg)
	if h == nil || h.instance == nil {
		t.Fatalf("expected GetOrCreate to return a handle backed by a pooled instance")
	}
	key := tripleKeyFromConfig(cfg)
	entry, ok := registry.entries[key]
	if !ok {
		t.Fatalf("expected a pooled entry to be registered for the schema identity")
	}
	if entry.refcount != 1 {
		t.Fatalf("expected refcount 1 for a freshly created entry, got %d", entry.refcount)
	}
}
