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

// Lab (508-lab, see .scratch/cisco-ios-xr-schema-collision/INDEX.md): two
// intents that each set a colliding "router" container defined by a
// different sibling YANG module (the Cisco-IOS-XR-um-* /router pattern —
// see ticket 03) must round-trip through JSON import, tree identity
// (NodeIdentity/ChildMap), and permodule.Encode as two distinct
// Set-encoded Updates, each carrying its own module's Path.Origin.
package permodule_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/permodule"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// collisionSchemaClient serves a schema for a "router" container that is
// defined identically by two sibling modules (mod-a, mod-b), each with a
// single distinguishing "foo" leaf — mirroring the Cisco IOS-XR sibling-module
// collision pattern surveyed in ticket 03, minimized to the smallest
// reproducible shape.
func collisionSchemaMock(t *testing.T, ctrl *gomock.Controller) schemaClient.SchemaClientBound {
	t.Helper()
	mockSc := mockschema.NewMockClient(ctrl)
	mockSc.EXPECT().GetSchema(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *sdcpb.GetSchemaRequest, _ ...interface{}) (*sdcpb.GetSchemaResponse, error) {
			path := req.GetPath()
			elems := path.GetElem()
			if len(elems) == 0 {
				// Root's own schema lookup (nil/empty path): schema-server
				// returns the synthetic "__root__" container whose children
				// are module names — see pkg/utils/converter.go's __root__
				// handling. Without this, root.GetSchema() stays nil and
				// GetFirstAncestorWithSchema never finds a schema-bearing
				// ancestor for direct root children.
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Container{
							Container: &sdcpb.ContainerSchema{Name: "__root__"},
						},
					},
				}, nil
			}

			mod := path.GetOrigin()
			if mod == "" {
				if m, _, ok := strings.Cut(elems[0].GetName(), ":"); ok {
					mod = m
				}
			}
			if mod == "" {
				mod = "mod-a"
			}

			last := elems[len(elems)-1].GetName()
			if _, local, ok := strings.Cut(last, ":"); ok {
				last = local
			}

			switch last {
			case "router":
				return &sdcpb.GetSchemaResponse{
					Schema: &sdcpb.SchemaElem{
						Schema: &sdcpb.SchemaElem_Container{
							Container: &sdcpb.ContainerSchema{Name: "router", ModuleName: mod},
						},
					},
				}, nil
			case "foo":
				return &sdcpb.GetSchemaResponse{Schema: fooLeafSchemaElem(mod)}, nil
			}
			return &sdcpb.GetSchemaResponse{}, nil
		},
	).AnyTimes()

	schemaCfg := &config.SchemaConfig{Name: "test", Vendor: "v", Version: "1"}
	return schemaClient.NewSchemaClientBound(schemaCfg, mockSc)
}

func fooLeafSchemaElem(mod string) *sdcpb.SchemaElem {
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Name:       "foo",
				ModuleName: mod,
				Type:       &sdcpb.SchemaLeafType{Type: "string"},
			},
		},
	}
}

// TestLab_CollidingRouterIntents_ProduceTwoSetOrigins is the 508-lab exit-bar
// test: two owners each set "router" from a different module
// (mod-a:router, mod-b:router); the encoded merge Set plan must contain two
// Updates, one per module, both rooted at Path.Elem[0].Name == "router" but
// with distinct Path.Origin, and JSON content that was not conflated between
// the two owners.
func TestLab_CollidingRouterIntents_ProduceTwoSetOrigins(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	scb := collisionSchemaMock(t, ctrl)

	ctx := context.Background()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	// Intent 1: owner "isis-owner" sets mod-a:router/foo=isis.
	dataA := map[string]any{"mod-a:router": map[string]any{"foo": "isis"}}
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(dataA, "isis-owner", 5, false), types.NewUpdateInsertFlags().SetNewFlag(), vpf); err != nil {
		t.Fatalf("import mod-a intent: %v", err)
	}

	// Intent 2: owner "ospf-owner" sets mod-b:router/foo=ospf — same local
	// container name "router", different module.
	dataB := map[string]any{"mod-b:router": map[string]any{"foo": "ospf"}}
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(dataB, "ospf-owner", 5, false), types.NewUpdateInsertFlags().SetNewFlag(), vpf); err != nil {
		t.Fatalf("import mod-b intent: %v", err)
	}

	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("FinishInsertionPhase: %v", err)
	}

	plan, err := permodule.Encode(ctx, scb, root.Entry, gnmi.Encoding_JSON_IETF, false)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if len(plan.Updates) != 2 {
		t.Fatalf("want 2 Updates (one per colliding module), got %d: %v", len(plan.Updates), plan.Updates)
	}

	gotByOrigin := map[string][]byte{}
	for _, u := range plan.Updates {
		p := u.GetPath()
		if len(p.GetElem()) != 1 || p.GetElem()[0].GetName() != "router" {
			t.Fatalf("Update path %v: want exactly [router]", p)
		}
		origin := p.GetOrigin()
		if origin == "" {
			t.Fatalf("Update for path %v has empty Path.Origin", p)
		}
		gotByOrigin[origin] = u.GetValue().GetJsonIetfVal()
	}

	if len(gotByOrigin) != 2 {
		t.Fatalf("want 2 distinct Path.Origin values, got %v", gotByOrigin)
	}

	wantByOrigin := map[string]string{"mod-a": "isis", "mod-b": "ospf"}
	for mod, wantFoo := range wantByOrigin {
		raw, ok := gotByOrigin[mod]
		if !ok {
			t.Fatalf("missing Update with Path.Origin=%q; got origins: %v", mod, keysOf(gotByOrigin))
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal JSON for origin %q: %v", mod, err)
		}
		if got["foo"] != wantFoo {
			t.Errorf("origin %q: foo = %v, want %q (owners' payloads must not be conflated)", mod, got["foo"], wantFoo)
		}
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
