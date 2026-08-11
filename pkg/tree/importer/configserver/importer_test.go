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
	"testing"

	"github.com/google/go-cmp/cmp"
	csreader "github.com/sdcio/data-server/pkg/cache/configserver"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/testing/protocmp"
)

// TestNewImportAdapter_FieldMapping covers the ADR's field-mapping table end
// to end: every ImportConfigAdapter accessor must reflect the Document field
// it's mapped from.
func TestNewImportAdapter_FieldMapping(t *testing.T) {
	sensitivePaths := []*sdcpb.Path{
		{Elem: []*sdcpb.PathElem{{Name: "secret"}}},
	}
	doc := &csreader.Document{
		Name:           "intent1",
		Priority:       10,
		NonRevertive:   true,
		Orphan:         true,
		SensitivePaths: sensitivePaths,
		Config: []*csreader.ConfigBlob{
			{Path: "/interface[name=eth0]/description", Value: []byte(`"uplink"`)},
		},
	}

	adapter, err := NewImportAdapter(doc)
	if err != nil {
		t.Fatalf("NewImportAdapter() error = %v", err)
	}

	if got := adapter.GetName(); got != doc.Name {
		t.Errorf("GetName() = %q, want %q", got, doc.Name)
	}
	if got := adapter.GetPriority(); got != doc.Priority {
		t.Errorf("GetPriority() = %d, want %d", got, doc.Priority)
	}
	if got := adapter.GetNonRevertive(); got != doc.NonRevertive {
		t.Errorf("GetNonRevertive() = %v, want %v", got, doc.NonRevertive)
	}
	if got := adapter.GetOrphan(); got != doc.Orphan {
		t.Errorf("GetOrphan() = %v, want %v", got, doc.Orphan)
	}
	if diff := cmp.Diff(sensitivePaths, adapter.GetSensitivePaths(), protocmp.Transform()); diff != "" {
		t.Errorf("GetSensitivePaths() mismatch (-want +got):\n%s", diff)
	}
	if adapter.GetDeletes() == nil {
		t.Error("GetDeletes() = nil, want empty PathSet (config-server-owned intents never carry ExplicitDeletes)")
	}

	elem := adapter.GetElement("interface")
	if elem == nil {
		t.Fatal("GetElement(\"interface\") = nil, want the merged config payload traversable")
	}
}

// TestNewImportAdapter_DefaultsWithoutSeeding verifies zero-valued Document
// fields (no orphan, no sensitive paths) round-trip as false/nil, per the
// ADR — these aren't errors, just the steady-state case for most intents.
func TestNewImportAdapter_DefaultsWithoutSeeding(t *testing.T) {
	adapter, err := NewImportAdapter(&csreader.Document{Name: "intent1"})
	if err != nil {
		t.Fatalf("NewImportAdapter() error = %v", err)
	}
	if adapter.GetOrphan() {
		t.Error("GetOrphan() = true, want false")
	}
	if adapter.GetSensitivePaths() != nil {
		t.Errorf("GetSensitivePaths() = %v, want nil", adapter.GetSensitivePaths())
	}
}

func TestNewImportAdapter_InvalidConfigPropagatesError(t *testing.T) {
	_, err := NewImportAdapter(&csreader.Document{
		Name:   "intent1",
		Config: []*csreader.ConfigBlob{{Path: "[invalid", Value: []byte(`1`)}},
	})
	if err == nil {
		t.Fatal("NewImportAdapter() expected error for invalid config, got nil")
	}
}

func TestNewImportAdapter_ImplementsInterface(t *testing.T) {
	adapter, err := NewImportAdapter(&csreader.Document{Name: "intent1"})
	if err != nil {
		t.Fatalf("NewImportAdapter() error = %v", err)
	}
	var _ importer.ImportConfigAdapter = adapter //nolint:staticcheck // explicit interface assertion is the point of this test
}
