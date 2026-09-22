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

package configsnapshot

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/testing/protocmp"
)

// TestRoundTrip_IntentDocumentAdapter is the primary seam for this package:
// Intent → Document → IntentAdapter must preserve metadata and tree
// content that DocumentFromIntent and NewImportAdapter share.
func TestRoundTrip_IntentDocumentAdapter(t *testing.T) {
	sensitive := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "secret"}}}}
	intent := &tree_persist.Intent{
		IntentName:     "ns1.intent1",
		Priority:       7,
		NonRevertive:   true,
		Orphan:         true,
		SensitivePaths: sensitive,
		Root: &tree_persist.TreeElement{
			Childs: []*tree_persist.TreeElement{
				{Name: "system", Childs: []*tree_persist.TreeElement{
					{Name: "config", Childs: []*tree_persist.TreeElement{
						{Name: "hostname", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "router1"}})},
					}},
				}},
				{Name: "interface", Childs: []*tree_persist.TreeElement{
					{Name: "name", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "eth0"}})},
					{Name: "mtu", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 9000}})},
				}},
				{Name: "interface", Childs: []*tree_persist.TreeElement{
					{Name: "name", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "eth1"}})},
					{Name: "mtu", LeafVariant: leafVariant(t, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 1500}})},
				}},
			},
		},
	}

	doc, err := DocumentFromIntent(Target{Namespace: "ns1", Name: "target1"}, "intent1", intent)
	if err != nil {
		t.Fatalf("DocumentFromIntent: %v", err)
	}

	adapter, err := NewImportAdapter(doc)
	if err != nil {
		t.Fatalf("NewImportAdapter: %v", err)
	}

	if got := adapter.GetName(); got != "ns1.intent1" {
		t.Errorf("GetName() = %q, want ns1.intent1", got)
	}
	if got := adapter.GetPriority(); got != 7 {
		t.Errorf("GetPriority() = %d, want 7", got)
	}
	if !adapter.GetNonRevertive() {
		t.Error("GetNonRevertive() = false, want true")
	}
	if !adapter.GetOrphan() {
		t.Error("GetOrphan() = false, want true")
	}
	if diff := cmp.Diff(sensitive, adapter.GetSensitivePaths(), protocmp.Transform()); diff != "" {
		t.Errorf("GetSensitivePaths() mismatch (-want +got):\n%s", diff)
	}

	system := adapter.GetElement("system")
	if system == nil {
		t.Fatal(`GetElement("system") = nil`)
	}
	config := system.GetElement("config")
	if config == nil {
		t.Fatal(`GetElement("system").GetElement("config") = nil`)
	}
	if config.GetElement("hostname") == nil {
		t.Fatal(`GetElement("hostname") = nil`)
	}
	hostnameVal, err := config.GetElement("hostname").GetKeyValue(t.Context(), nil)
	if err != nil {
		t.Fatalf("hostname GetKeyValue: %v", err)
	}
	if hostnameVal != "router1" {
		t.Errorf("hostname = %q, want router1", hostnameVal)
	}

	// List instances expand at the parent GetElements() level (same-named
	// siblings under one JSON array key), not as children of GetElement("interface").
	var ifaces int
	for _, e := range adapter.GetElements() {
		if e.GetName() == "interface" {
			ifaces++
		}
	}
	if ifaces != 2 {
		t.Fatalf("interface entries via GetElements() = %d, want 2", ifaces)
	}
}
