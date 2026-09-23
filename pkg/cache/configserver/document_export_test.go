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

package configserver

import (
	"encoding/json"
	"testing"

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

// TestDocumentFromIntent_singleLeaf covers the simplest case: one scalar
// leaf directly under the root, no containers, no lists.
func TestDocumentFromIntent_singleLeaf(t *testing.T) {
	intent := &tree_persist.Intent{
		IntentName: "ns1.intent1",
		Priority:   10,
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				{Name: "hostname", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "router1"}})},
			},
		},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}
	if doc.Name != "intent1" || doc.Namespace != "ns1" || doc.Priority != 10 {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.Config) != 1 || doc.Config[0].Path != "/" {
		t.Fatalf("Config = %+v, want a single root blob", doc.Config)
	}

	var got map[string]any
	if err := json.Unmarshal(doc.Config[0].Value, &got); err != nil {
		t.Fatalf("unmarshal root blob: %v", err)
	}
	if got["hostname"] != "router1" {
		t.Errorf("root blob = %+v, want hostname=router1", got)
	}
}

// TestDocumentFromIntent_nestedContainer covers a leaf nested two levels
// deep under plain (non-list) containers.
func TestDocumentFromIntent_nestedContainer(t *testing.T) {
	intent := &tree_persist.Intent{
		IntentName: "ns1.intent1",
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				{Name: "system", Childs: []*tree_persist.TreeElement{
					{Name: "config", Childs: []*tree_persist.TreeElement{
						{Name: "hostname", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "router1"}})},
					}},
				}},
			},
		},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(doc.Config[0].Value, &got); err != nil {
		t.Fatalf("unmarshal root blob: %v", err)
	}
	system, ok := got["system"].(map[string]any)
	if !ok {
		t.Fatalf("root blob = %+v, want a system container", got)
	}
	config, ok := system["config"].(map[string]any)
	if !ok {
		t.Fatalf("system = %+v, want a config container", system)
	}
	if config["hostname"] != "router1" {
		t.Errorf("config = %+v, want hostname=router1", config)
	}
}

// TestDocumentFromIntent_listEntriesGroupIntoArray covers TreeExport's list
// encoding: repeated same-named siblings under a list container's parent
// must render as a JSON array, matching what mergeConfigBlobs/
// JsonTreeImporter already expect on read.
func TestDocumentFromIntent_listEntriesGroupIntoArray(t *testing.T) {
	mkInterface := func(name string, mtu int64) *tree_persist.TreeElement {
		return &tree_persist.TreeElement{
			Name: "interface",
			Childs: []*tree_persist.TreeElement{
				{Name: "name", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: name}})},
				{Name: "mtu", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: uint64(mtu)}})},
			},
		}
	}
	intent := &tree_persist.Intent{
		IntentName: "ns1.intent1",
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				mkInterface("eth0", 1500),
				mkInterface("eth1", 9000),
			},
		},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(doc.Config[0].Value, &got); err != nil {
		t.Fatalf("unmarshal root blob: %v", err)
	}
	ifaces, ok := got["interface"].([]any)
	if !ok || len(ifaces) != 2 {
		t.Fatalf("interface = %+v, want a 2-element array", got["interface"])
	}
}

// TestDocumentFromIntent_singleListEntryStaysUngrouped covers the
// single-instance-list case: TreeExport gives no signal distinguishing a
// lone list entry from a plain container sharing that name, so it renders
// as a bare object — tolerated identically to a one-element array by
// JsonTreeImporter.GetElements on the read side.
func TestDocumentFromIntent_singleListEntryStaysUngrouped(t *testing.T) {
	intent := &tree_persist.Intent{
		IntentName: "ns1.intent1",
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				{Name: "interface", Childs: []*tree_persist.TreeElement{
					{Name: "name", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "eth0"}})},
				}},
			},
		},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(doc.Config[0].Value, &got); err != nil {
		t.Fatalf("unmarshal root blob: %v", err)
	}
	if _, isObject := got["interface"].(map[string]any); !isObject {
		t.Errorf("interface = %+v (%T), want a bare object for a single instance", got["interface"], got["interface"])
	}
}

// TestDocumentFromIntent_fieldMapping locks the incidental-field mapping off
// the Intent itself, independent of tree content.
func TestDocumentFromIntent_fieldMapping(t *testing.T) {
	intent := &tree_persist.Intent{
		IntentName:     "ns1.intent1",
		Priority:       7,
		NonRevertive:   true,
		Orphan:         true,
		SensitivePaths: []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "secret"}}}},
		Root:           &tree_persist.TreeElement{Childs: []*tree_persist.TreeElement{{Name: "x", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}})}}},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}
	if doc.Priority != 7 || !doc.NonRevertive || !doc.Orphan {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.SensitivePaths) != 1 {
		t.Errorf("SensitivePaths = %+v", doc.SensitivePaths)
	}
}

// TestDocumentFromIntent_nilRoot covers an Intent with no Root at all (e.g.
// an explicit-deletes-only export) — must not panic, and produces a Config
// list callers can still send onward without crashing.
func TestDocumentFromIntent_nilRoot(t *testing.T) {
	intent := &tree_persist.Intent{IntentName: "ns1.intent1"}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}
	if len(doc.Config) != 1 {
		t.Fatalf("Config = %+v", doc.Config)
	}
}
