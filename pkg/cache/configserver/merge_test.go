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
)

func TestMergeConfigBlobs_SimpleLeaf(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/description", Value: []byte(`"top level"`)},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	want := map[string]any{"description": "top level"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeConfigBlobs() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeConfigBlobs_NestedContainer(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/system/config/hostname", Value: []byte(`"router1"`)},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	want := map[string]any{
		"system": map[string]any{
			"config": map[string]any{
				"hostname": "router1",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeConfigBlobs() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeConfigBlobs_ListEntry_KeySeeded(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/interface[name=eth0]/config/mtu", Value: []byte(`9000`)},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	want := map[string]any{
		"interface": []any{
			map[string]any{
				"name": "eth0",
				"config": map[string]any{
					"mtu": float64(9000),
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeConfigBlobs() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeConfigBlobs_MultipleBlobsConvergeOnSameListEntry(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/interface[name=eth0]/config/mtu", Value: []byte(`9000`)},
		{Path: "/interface[name=eth0]/config/description", Value: []byte(`"uplink"`)},
		{Path: "/interface[name=eth1]/config/mtu", Value: []byte(`1500`)},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	want := map[string]any{
		"interface": []any{
			map[string]any{
				"name": "eth0",
				"config": map[string]any{
					"mtu":         float64(9000),
					"description": "uplink",
				},
			},
			map[string]any{
				"name": "eth1",
				"config": map[string]any{
					"mtu": float64(1500),
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeConfigBlobs() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeConfigBlobs_MultiKeyList(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/neighbor[local-as=65001][peer-as=65002]/description", Value: []byte(`"peer"`)},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	want := map[string]any{
		"neighbor": []any{
			map[string]any{
				"local-as":    "65001",
				"peer-as":     "65002",
				"description": "peer",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mergeConfigBlobs() mismatch (-want +got):\n%s", diff)
	}
}

func TestMergeConfigBlobs_NoValue(t *testing.T) {
	got, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/enabled"},
	})
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	if got["enabled"] != nil {
		t.Errorf("got[\"enabled\"] = %v, want nil", got["enabled"])
	}
}

func TestMergeConfigBlobs_Empty(t *testing.T) {
	got, err := mergeConfigBlobs(nil)
	if err != nil {
		t.Fatalf("mergeConfigBlobs() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("mergeConfigBlobs(nil) = %v, want empty", got)
	}
}

func TestMergeConfigBlobs_InvalidPath(t *testing.T) {
	_, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "[invalid", Value: []byte(`1`)},
	})
	if err == nil {
		t.Fatal("mergeConfigBlobs() expected error for invalid path, got nil")
	}
}

func TestMergeConfigBlobs_InvalidJSON(t *testing.T) {
	_, err := mergeConfigBlobs([]*ConfigBlob{
		{Path: "/mtu", Value: []byte(`not-json`)},
	})
	if err == nil {
		t.Fatal("mergeConfigBlobs() expected error for invalid JSON, got nil")
	}
}
