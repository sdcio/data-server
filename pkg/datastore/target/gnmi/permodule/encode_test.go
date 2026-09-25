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

package permodule_test

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/gnmi/proto/gnmi"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/permodule"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// newTestRoot builds a tree root wired to the test schema and returns the root
// and the schema client bound to it (for passing to Encode).
func newTestRoot(t *testing.T, mockCtrl *gomock.Controller) (*tree.RootEntry, schemaClient.SchemaClientBound) {
	t.Helper()
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	return root, scb
}

// interfaceUpdates returns a single interface entry update.
// name must match the sdcio_model_if interface-name pattern (e.g. "ethernet-1/1").
func interfaceUpdates(name, desc string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "interface", Key: map[string]string{"name": name}},
			{Name: "description"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
	}}
}

// networkInstanceUpdates returns a single network-instance description update (module sdcio_model_ni).
func networkInstanceUpdates(name, desc string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "network-instance", Key: map[string]string{"name": name}},
			{Name: "description"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
	}}
}

// addToRoot adds updates with the given flags.
func addToRoot(t *testing.T, root *tree.RootEntry, updates []*sdcpb.Update, flags *types.UpdateInsertFlags) {
	t.Helper()
	ctx := context.Background()
	if err := testhelper.AddToRoot(ctx, root.Entry, updates, flags, "owner1", 5); err != nil {
		t.Fatal(err)
	}
}

func finish(t *testing.T, root *tree.RootEntry) {
	t.Helper()
	if err := root.FinishInsertionPhase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// originSet collects Path.Origin values from a list of Updates.
func originSet(updates []*sdcpb.Update) map[string]bool {
	m := make(map[string]bool, len(updates))
	for _, u := range updates {
		m[u.GetPath().GetOrigin()] = true
	}
	return m
}

// deleteOriginSet collects Origin values from a list of Paths.
func deleteOriginSet(paths []*sdcpb.Path) map[string]bool {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		m[p.GetOrigin()] = true
	}
	return m
}

// --- Cycle 1: multi-module merge emits one Update per module ----------

func TestEncode_MultiModuleMerge_TwoUpdates(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	upds := append(interfaceUpdates("ethernet-1/1", "uplink"), networkInstanceUpdates("default", "Default NI")...)
	addToRoot(t, root, upds, testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(context.Background(), scb, root.Entry, gnmi.Encoding_JSON_IETF, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Updates) != 2 {
		t.Fatalf("want 2 Updates (one per module), got %d", len(plan.Updates))
	}
	origins := originSet(plan.Updates)
	if !origins["sdcio_model_if"] {
		t.Errorf("missing Update for module sdcio_model_if; origins: %v", origins)
	}
	if !origins["sdcio_model_ni"] {
		t.Errorf("missing Update for module sdcio_model_ni; origins: %v", origins)
	}
	for _, u := range plan.Updates {
		if len(u.GetPath().GetElem()) != 1 {
			t.Errorf("Update for %q: want exactly 1 path elem, got %d",
				u.GetPath().GetOrigin(), len(u.GetPath().GetElem()))
		}
	}
	if len(plan.Deletes) != 0 {
		t.Errorf("merge with no deletes: want 0 Deletes, got %d", len(plan.Deletes))
	}
}

// --- Cycle 2: single-module tree emits exactly 1 Update ---------------

func TestEncode_SingleModuleMerge_OneUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addToRoot(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(context.Background(), scb, root.Entry, gnmi.Encoding_JSON_IETF, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	u := plan.Updates[0]
	if u.GetPath().GetOrigin() != "sdcio_model_if" {
		t.Errorf("want origin sdcio_model_if, got %q", u.GetPath().GetOrigin())
	}
	if len(u.GetPath().GetElem()) != 1 || u.GetPath().GetElem()[0].GetName() != "interface" {
		t.Errorf("want Path.Elem[0].Name=interface, got %v", u.GetPath().GetElem())
	}
}

// --- Cycle 3: unchanged module is omitted in merge --------------------

func TestEncode_MergeOmitsModuleWithNoNewLeaves(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	ctx := context.Background()
	// Running config carries the NI — the device already has it.
	// We use RunningValuesPrio / RunningIntentName so the tree considers it
	// "equal to running" and omits it when onlyNewOrUpdated=true.
	niUpd := networkInstanceUpdates("default", "Default NI")
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	// Intent also carries the same NI (same value) so it is NOT new.
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	// Only the interface entry is new.
	addToRoot(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(ctx, scb, root.Entry, gnmi.Encoding_JSON_IETF, false)
	if err != nil {
		t.Fatal(err)
	}

	// Only the interface module has new/updated leaves.
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update (interface only), got %d", len(plan.Updates))
	}
	if plan.Updates[0].GetPath().GetOrigin() != "sdcio_model_if" {
		t.Errorf("want sdcio_model_if, got %q", plan.Updates[0].GetPath().GetOrigin())
	}
}

// --- Cycle 4: replace emits per-module delete then update -------------

func TestEncode_MultiModuleReplace_DeletesAndUpdates(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	upds := append(interfaceUpdates("ethernet-1/1", "uplink"), networkInstanceUpdates("default", "Default NI")...)
	addToRoot(t, root, upds, testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(context.Background(), scb, root.Entry, gnmi.Encoding_JSON_IETF, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Updates) != 2 {
		t.Fatalf("want 2 Updates, got %d", len(plan.Updates))
	}
	if len(plan.Deletes) != 2 {
		t.Fatalf("want 2 Deletes (one per module), got %d", len(plan.Deletes))
	}

	dOrigins := deleteOriginSet(plan.Deletes)
	if !dOrigins["sdcio_model_if"] {
		t.Errorf("missing Delete for sdcio_model_if; delete origins: %v", dOrigins)
	}
	if !dOrigins["sdcio_model_ni"] {
		t.Errorf("missing Delete for sdcio_model_ni; delete origins: %v", dOrigins)
	}
	for _, d := range plan.Deletes {
		if len(d.GetElem()) != 1 {
			t.Errorf("Delete for %q: want 1 path elem, got %d", d.GetOrigin(), len(d.GetElem()))
		}
	}

	uOrigins := originSet(plan.Updates)
	if diff := cmp.Diff(dOrigins, uOrigins); diff != "" {
		t.Errorf("Update and Delete origin sets differ (-delete +update):\n%s", diff)
	}
}

// --- Cycle 5: merge deletes carry Path.Origin -------------------------

func TestEncode_MergeDeletes_OriginIsSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	ctx := context.Background()
	// The running config has the NI — the device already has it.
	niUpd := networkInstanceUpdates("default", "Default NI")
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	// The intent owner marks the same NI description for deletion (with the original
	// value preserved so the serializer does not encounter a nil TypedValue).
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsDelete, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	// Also add a new interface entry so we have at least one Update.
	addToRoot(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(ctx, scb, root.Entry, gnmi.Encoding_JSON_IETF, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Updates) < 1 {
		t.Fatalf("want ≥1 Update, got %d", len(plan.Updates))
	}
	if len(plan.Deletes) < 1 {
		t.Fatalf("want ≥1 Delete (the NI deletion), got 0")
	}
	for _, d := range plan.Deletes {
		if d.GetOrigin() == "" {
			t.Errorf("Delete path %v has empty Origin", d)
		}
	}
}

// --- Bonus: JSON content validation -----------------------------------

func TestEncode_JSONContent_MatchesLeafValues(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addToRoot(t, root, networkInstanceUpdates("default", "Default NI"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := permodule.Encode(context.Background(), scb, root.Entry, gnmi.Encoding_JSON, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}

	jsonBytes := plan.Updates[0].GetValue().GetJsonVal()
	if jsonBytes == nil {
		t.Fatal("expected JsonVal, got nil")
	}

	var got any
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("unmarshal JSON value: %v", err)
	}
	t.Logf("encoded JSON:\n%s", func() string { b, _ := json.MarshalIndent(got, "", "  "); return string(b) }())
}
