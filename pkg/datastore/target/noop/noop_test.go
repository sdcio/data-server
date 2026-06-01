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

package noop

import (
	"context"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/datastore/target/types"
)

func TestNoopTarget_Set_GnmiPlan_ReturnsUpdateResultsForUpdatesAndDeletes(t *testing.T) {
	nt, err := NewNoopTarget(context.Background(), "test")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	plan := types.SouthboundSetPlan{
		Gnmi: &types.GnmiSetPlan{
			Updates: []*sdcpb.Update{
				{Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}}},
				{Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "bgp"}}}},
			},
			Deletes: []*sdcpb.Path{
				{Elem: []*sdcpb.PathElem{{Name: "routing"}}},
			},
		},
	}

	rsp, err := nt.Set(context.Background(), plan)
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if rsp == nil {
		t.Fatal("Set: expected non-nil response")
	}

	wantOps := []sdcpb.UpdateResult_Operation{
		sdcpb.UpdateResult_UPDATE,
		sdcpb.UpdateResult_UPDATE,
		sdcpb.UpdateResult_DELETE,
	}
	if len(rsp.Response) != len(wantOps) {
		t.Fatalf("Set: expected %d response entries, got %d", len(wantOps), len(rsp.Response))
	}
	for i, want := range wantOps {
		if rsp.Response[i].Op != want {
			t.Errorf("entry %d: want Op %v, got %v", i, want, rsp.Response[i].Op)
		}
	}
}

func TestNoopTarget_Set_NetconfPlan_ReturnsEmptyResponse(t *testing.T) {
	nt, err := NewNoopTarget(context.Background(), "test")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	plan := types.SouthboundSetPlan{
		Netconf: &types.NetconfSetPlan{},
	}

	rsp, err := nt.Set(context.Background(), plan)
	if err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	if rsp == nil {
		t.Fatal("Set: expected non-nil response")
	}
}
