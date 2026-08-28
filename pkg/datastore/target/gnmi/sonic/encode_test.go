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

package sonic_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/sonic"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

const sonicOrigin = "sonic_yang"

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

func addToRoot(t *testing.T, root *tree.RootEntry, updates []*sdcpb.Update, flags *types.UpdateInsertFlags) {
	t.Helper()
	if err := testhelper.AddToRoot(context.Background(), root.Entry, updates, flags, "owner1", 5); err != nil {
		t.Fatal(err)
	}
}

func finish(t *testing.T, root *tree.RootEntry) {
	t.Helper()
	if err := root.FinishInsertionPhase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func pathElemsXPath(p *sdcpb.Path) string {
	return (&sdcpb.Path{Elem: p.GetElem()}).ToXPath(false)
}

func seedNetworkInstance(t *testing.T, root *tree.RootEntry) {
	t.Helper()
	ctx := context.Background()
	niUpd := networkInstanceDescriptionUpdate("default", "Default NI")
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting, "owner1", 5); err != nil {
		t.Fatal(err)
	}
}

func updateJSON(t *testing.T, u *sdcpb.Update) map[string]any {
	t.Helper()
	b := u.GetValue().GetJsonIetfVal()
	if b == nil {
		t.Fatal("expected JsonIetfVal")
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal update JSON: %v", err)
	}
	return got
}

func assertNoRFC7951KeyPrefixes(t *testing.T, v any) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if strings.Contains(k, ":") {
				t.Errorf("JSON key %q still has RFC 7951 module prefix", k)
			}
			assertNoRFC7951KeyPrefixes(t, val)
		}
	case []any:
		for _, val := range x {
			assertNoRFC7951KeyPrefixes(t, val)
		}
	}
}

func interfaceDescriptionUpdate(name, desc string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "interface", Key: map[string]string{"name": name}},
			{Name: "description"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
	}}
}

func interfaceMultiLeafUpdates(name, desc, adminState string) []*sdcpb.Update {
	return []*sdcpb.Update{
		{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "interface", Key: map[string]string{"name": name}},
				{Name: "description"},
			}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
		},
		{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "interface", Key: map[string]string{"name": name}},
				{Name: "admin-state"},
			}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: adminState}},
		},
	}
}

func bgpLeafUpdates(niName string, asn uint64, routerID string) []*sdcpb.Update {
	return []*sdcpb.Update{
		{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "network-instance", Key: map[string]string{"name": niName}},
				{Name: "protocol"},
				{Name: "bgp"},
				{Name: "autonomous-system"},
			}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: asn}},
		},
		{
			Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "network-instance", Key: map[string]string{"name": niName}},
				{Name: "protocol"},
				{Name: "bgp"},
				{Name: "router-id"},
			}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: routerID}},
		},
	}
}

func nestedNIInterfaceUpdate(niName, ifName string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "network-instance", Key: map[string]string{"name": niName}},
			{Name: "interface", Key: map[string]string{"name": ifName}},
			{Name: "name"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: ifName}},
	}}
}

func doublekeyMandatoUpdate(key2, key1, mandato string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "doublekey", Key: map[string]string{"key2": key2, "key1": key1}},
			{Name: "mandato"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: mandato}},
	}}
}

func networkInstanceDescriptionUpdate(name, desc string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "network-instance", Key: map[string]string{"name": name}},
			{Name: "description"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
	}}
}

func TestEncode_SingleLeafPlainContainer_OneUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	seedNetworkInstance(t, root)
	addToRoot(t, root, bgpLeafUpdates("default", 65001, "192.0.2.1")[:1], testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	u := plan.Updates[0]
	if u.GetPath().GetOrigin() != sonicOrigin {
		t.Errorf("want origin %q, got %q", sonicOrigin, u.GetPath().GetOrigin())
	}
	wantPath := "network-instance[name=default]/protocol/bgp"
	if got := pathElemsXPath(u.GetPath()); got != wantPath {
		t.Errorf("path: got %q, want %q", got, wantPath)
	}
	body := updateJSON(t, u)
	if _, ok := body["autonomous-system"]; !ok {
		t.Errorf("expected autonomous-system in JSON body, got %v", body)
	}
}

func TestEncode_MultipleSiblingLeavesPlainContainer_Batched(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	seedNetworkInstance(t, root)
	addToRoot(t, root, bgpLeafUpdates("default", 65001, "192.0.2.1"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 batched Update, got %d", len(plan.Updates))
	}
	body := updateJSON(t, plan.Updates[0])
	if _, ok := body["autonomous-system"]; !ok {
		t.Error("missing autonomous-system")
	}
	if _, ok := body["router-id"]; !ok {
		t.Error("missing router-id")
	}
}

func TestEncode_NewKeyedListRow_FullRowWrap(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addToRoot(t, root, interfaceDescriptionUpdate("ethernet-1/1", "uplink"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	u := plan.Updates[0]
	if pathElemsXPath(u.GetPath()) != "interface[name=ethernet-1/1]" {
		t.Errorf("unexpected path: %s", pathElemsXPath(u.GetPath()))
	}
	body := updateJSON(t, u)
	ifaceList, ok := body["interface"].([]any)
	if !ok || len(ifaceList) != 1 {
		t.Fatalf("want array-wrapped interface row, got %v", body["interface"])
	}
	row, ok := ifaceList[0].(map[string]any)
	if !ok {
		t.Fatalf("row is not an object: %T", ifaceList[0])
	}
	if row["description"] != "uplink" {
		t.Errorf("row description: got %v", row["description"])
	}
	if row["name"] != "ethernet-1/1" {
		t.Errorf("row name: got %v", row["name"])
	}
}

func TestEncode_ExistingKeyedListRowChange_FullRowWrap(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	ctx := context.Background()

	existing := interfaceMultiLeafUpdates("ethernet-1/1", "old-desc", "enable")
	if err := testhelper.AddToRoot(ctx, root.Entry, existing, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	addToRoot(t, root, interfaceDescriptionUpdate("ethernet-1/1", "new-desc"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(ctx, scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	body := updateJSON(t, plan.Updates[0])
	ifaceList := body["interface"].([]any)
	row := ifaceList[0].(map[string]any)
	if row["description"] != "new-desc" {
		t.Errorf("description: got %v", row["description"])
	}
	if row["admin-state"] != "enable" {
		t.Errorf("expected full row to include unchanged admin-state, got %v", row["admin-state"])
	}
}

func TestEncode_MultiKeyListRow_FullRowWrap(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addToRoot(t, root, doublekeyMandatoUpdate("k2a", "k1a", "value"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	u := plan.Updates[0]
	lastElem := u.GetPath().GetElem()[len(u.GetPath().GetElem())-1]
	if lastElem.GetName() != "doublekey" || lastElem.GetKey()["key2"] != "k2a" || lastElem.GetKey()["key1"] != "k1a" {
		t.Errorf("unexpected path elem: %v", lastElem)
	}
	body := updateJSON(t, u)
	rows, ok := body["doublekey"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("want array-wrapped doublekey row keyed by the list name, got %v", body)
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("row is not an object: %T", rows[0])
	}
	if row["mandato"] != "value" {
		t.Errorf("row mandato: got %v", row["mandato"])
	}
	if row["key2"] != "k2a" || row["key1"] != "k1a" {
		t.Errorf("row keys: got key2=%v key1=%v", row["key2"], row["key1"])
	}
}

func TestEncode_NestedModuleContainerList_IndependentGrouping(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	ctx := context.Background()

	niUpd := networkInstanceDescriptionUpdate("default", "Default NI")
	addToRoot(t, root, niUpd, testhelper.FlagsNew)
	addToRoot(t, root, nestedNIInterfaceUpdate("default", "ethernet-1/1.0"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(ctx, scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 2 {
		t.Fatalf("want 2 Updates (NI row + nested interface row), got %d", len(plan.Updates))
	}
	paths := map[string]bool{}
	for _, u := range plan.Updates {
		paths[pathElemsXPath(u.GetPath())] = true
	}
	if !paths["network-instance[name=default]"] {
		t.Errorf("missing NI list-instance update; paths: %v", paths)
	}
	if !paths["network-instance[name=default]/interface[name=ethernet-1/1.0]"] {
		t.Errorf("missing nested interface list-instance update; paths: %v", paths)
	}
}

func TestEncode_RFC7951PrefixesStrippedRecursively(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	seedNetworkInstance(t, root)
	addToRoot(t, root, bgpLeafUpdates("default", 65001, "192.0.2.2"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	body := updateJSON(t, plan.Updates[0])
	assertNoRFC7951KeyPrefixes(t, body)
	if _, ok := body["autonomous-system"]; !ok {
		t.Errorf("expected nested unprefixed keys, got %v", body)
	}
}

func TestEncode_RFC7951PrefixesStrippedInCrossModuleListRow(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addToRoot(t, root, networkInstanceDescriptionUpdate("default", "Default NI"), testhelper.FlagsNew)
	finish(t, root)

	plan, err := sonic.Encode(context.Background(), scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("want 1 Update, got %d", len(plan.Updates))
	}
	body := updateJSON(t, plan.Updates[0])
	assertNoRFC7951KeyPrefixes(t, body)
	rows, ok := body["network-instance"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("want cross-module list wrap key network-instance, got %v", body)
	}
}

func TestEncode_DeletePassthrough(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	ctx := context.Background()

	niUpd := networkInstanceDescriptionUpdate("default", "Default NI")
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsDelete, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	finish(t, root)

	plan, err := sonic.Encode(ctx, scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 0 {
		t.Fatalf("want 0 Updates, got %d", len(plan.Updates))
	}
	if len(plan.Deletes) != 1 {
		t.Fatalf("want 1 Delete, got %d", len(plan.Deletes))
	}
	if plan.Deletes[0].GetOrigin() != "" {
		t.Errorf("delete should pass through unmodified (no origin rewrite), got %q", plan.Deletes[0].GetOrigin())
	}
}

func TestEncode_NoChanges_EmitsNothing(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	ctx := context.Background()

	niUpd := networkInstanceDescriptionUpdate("default", "Default NI")
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting,
		consts.RunningIntentName, consts.RunningValuesPrio); err != nil {
		t.Fatal(err)
	}
	if err := testhelper.AddToRoot(ctx, root.Entry, niUpd, testhelper.FlagsExisting, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	finish(t, root)

	plan, err := sonic.Encode(ctx, scb, root.Entry, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		return
	}
	if len(plan.Updates) != 0 || len(plan.Deletes) != 0 {
		t.Fatalf("want empty plan, got %d updates and %d deletes", len(plan.Updates), len(plan.Deletes))
	}
}
