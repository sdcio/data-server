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

package permodule

import (
	"context"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// lookupSchemaClient records the path passed to GetSchemaSdcpbPath.
type lookupSchemaClient struct {
	got *sdcpb.Path
}

func (l *lookupSchemaClient) GetSchemaSdcpbPath(_ context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	l.got = path
	return &sdcpb.GetSchemaResponse{
		Schema: &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Container{
				Container: &sdcpb.ContainerSchema{Name: "router", ModuleName: "mod-a"},
			},
		},
	}, nil
}

func (l *lookupSchemaClient) GetSchemaElements(context.Context, *sdcpb.Path, chan struct{}) (chan *sdcpb.GetSchemaResponse, error) {
	return nil, nil
}

func TestResolveDeleteOrigin_UsesPathOrigin(t *testing.T) {
	scb := &lookupSchemaClient{}
	del := treetypes.NewDeleteEntryImpl(&sdcpb.Path{
		Origin: "mod-a",
		Elem: []*sdcpb.PathElem{
			{Name: "router"},
			{Name: "isis"},
		},
	})

	origin, err := resolveDeleteOrigin(context.Background(), scb, del)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "mod-a" {
		t.Fatalf("origin %q want mod-a", origin)
	}
	assertLookup(t, scb.got, "mod-a", "router")
}

func TestResolveDeleteOrigin_ParsesModuleQualifiedFirstElem(t *testing.T) {
	scb := &lookupSchemaClient{}
	del := treetypes.NewDeleteEntryImpl(&sdcpb.Path{
		Elem: []*sdcpb.PathElem{{Name: "mod-a:router"}},
	})

	origin, err := resolveDeleteOrigin(context.Background(), scb, del)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "mod-a" {
		t.Fatalf("origin %q want mod-a", origin)
	}
	assertLookup(t, scb.got, "mod-a", "router")
}

func TestResolveDeleteOrigin_BarePathStaysUnqualified(t *testing.T) {
	scb := &lookupSchemaClient{}
	del := treetypes.NewDeleteEntryImpl(&sdcpb.Path{
		Elem: []*sdcpb.PathElem{{Name: "interface"}},
	})

	if _, err := resolveDeleteOrigin(context.Background(), scb, del); err != nil {
		t.Fatal(err)
	}
	assertLookup(t, scb.got, "", "interface")
}

func assertLookup(t *testing.T, path *sdcpb.Path, origin, local string) {
	t.Helper()
	if path.GetOrigin() != origin {
		t.Fatalf("lookup origin %q want %q", path.GetOrigin(), origin)
	}
	elems := path.GetElem()
	if len(elems) != 1 || elems[0].GetName() != local {
		t.Fatalf("lookup elems %v want [%s]", elems, local)
	}
}

// Ensure the fake satisfies the schema client used by Encode.
var _ schemaClient.SchemaClientBound = (*lookupSchemaClient)(nil)
