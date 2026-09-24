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

package materialize_test

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/materialize"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// newTestRoot creates an empty tree root backed by the real test schema and
// returns the schema client so tests can pass it to BuildPlan.
func newTestRoot(t *testing.T, mockCtrl *gomock.Controller) (*tree.RootEntry, schemaClient.SchemaClientBound) {
	t.Helper()
	ctx := context.Background()
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatalf("GetSchemaClientBound: %v", err)
	}
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, tp)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}
	return root, scb
}

// interfaceUpdates returns a single interface entry update.
func interfaceUpdates(name, desc string) []*sdcpb.Update {
	return []*sdcpb.Update{{
		Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "interface", Key: map[string]string{"name": name}},
			{Name: "description"},
		}},
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: desc}},
	}}
}

// addAndFinish inserts updates into the root and calls FinishInsertionPhase.
func addAndFinish(t *testing.T, root *tree.RootEntry, updates []*sdcpb.Update, flags *types.UpdateInsertFlags) {
	t.Helper()
	if err := testhelper.AddToRoot(context.Background(), root.Entry, updates, flags, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	if err := root.FinishInsertionPhase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPlan_DisabledProfiles_ReturnNotEnabled(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	addAndFinish(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)

	sbi := &config.SBI{
		Type:          config.SBITypeGnmi,
		DeviceProfile: config.DeviceProfileSonic,
		GnmiOptions:   &config.SBIGnmiOptions{Encoding: "JSON_IETF"},
	}
	_, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err == nil {
		t.Fatal("BuildPlan(sonic): expected error, got nil")
	}
	if !errors.Is(err, config.ErrDeviceProfileNotEnabled) {
		t.Fatalf("BuildPlan(sonic): expected ErrDeviceProfileNotEnabled, got %v", err)
	}
}

// --- Cisco IOS-XR gNMI materialization -----------------------------------

func TestBuildPlan_CiscoIOSXR_JsonIETF_PerModulePlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	upds := append(interfaceUpdates("ethernet-1/1", "uplink"), networkInstanceUpdates("default", "Default NI")...)
	addAndFinish(t, root, upds, testhelper.FlagsNew)

	sbi := &config.SBI{
		Type:          config.SBITypeGnmi,
		DeviceProfile: config.DeviceProfileCiscoIOSXR,
		GnmiOptions:   &config.SBIGnmiOptions{Encoding: "JSON_IETF"},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Gnmi == nil {
		t.Fatalf("BuildPlan: expected Gnmi plan, got nil")
	}
	if len(plan.Gnmi.Updates) != 2 {
		t.Fatalf("want 2 Updates (one per YANG module), got %d", len(plan.Gnmi.Updates))
	}
	origins := make(map[string]bool)
	for _, u := range plan.Gnmi.Updates {
		origins[u.GetPath().GetOrigin()] = true
		if u.GetValue().GetJsonIetfVal() == nil {
			t.Errorf("Update for %q: expected JsonIetfVal, got nil", u.GetPath().GetOrigin())
		}
	}
	if !origins["sdcio_model_if"] {
		t.Errorf("missing Update for module sdcio_model_if; origins: %v", origins)
	}
	if !origins["sdcio_model_ni"] {
		t.Errorf("missing Update for module sdcio_model_ni; origins: %v", origins)
	}
}

func TestBuildPlan_CiscoIOSXR_Proto_GenericPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addAndFinish(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)

	sbi := &config.SBI{
		Type:          config.SBITypeGnmi,
		DeviceProfile: config.DeviceProfileCiscoIOSXR,
		GnmiOptions:   &config.SBIGnmiOptions{Encoding: "PROTO"},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Gnmi == nil {
		t.Fatalf("BuildPlan: expected Gnmi plan, got nil")
	}
	for _, u := range plan.Gnmi.Updates {
		if u.GetPath().GetOrigin() != "" {
			t.Errorf("proto plan must not set Path.Origin, got %q", u.GetPath().GetOrigin())
		}
	}
}

func TestBuildPlan_CiscoIOSXR_JSON_GenericPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	upds := append(interfaceUpdates("ethernet-1/1", "uplink"), networkInstanceUpdates("default", "Default NI")...)
	addAndFinish(t, root, upds, testhelper.FlagsNew)

	sbi := &config.SBI{
		Type:          config.SBITypeGnmi,
		DeviceProfile: config.DeviceProfileCiscoIOSXR,
		GnmiOptions:   &config.SBIGnmiOptions{Encoding: "JSON"},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Gnmi == nil {
		t.Fatalf("BuildPlan: expected Gnmi plan, got nil")
	}
	if len(plan.Gnmi.Updates) != 1 {
		t.Fatalf("want 1 root-level Update (generic path), got %d", len(plan.Gnmi.Updates))
	}
	u := plan.Gnmi.Updates[0]
	if len(u.GetPath().GetElem()) != 0 {
		t.Errorf("generic plan must target the root path, got %v", u.GetPath().GetElem())
	}
	if u.GetPath().GetOrigin() != "" {
		t.Errorf("generic plan must not set Path.Origin, got %q", u.GetPath().GetOrigin())
	}
	if u.GetValue().GetJsonVal() == nil {
		t.Errorf("expected JsonVal, got nil")
	}
}

func TestBuildPlan_CiscoIOSXR_Netconf_ReturnsNetconfSetPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	sbi := &config.SBI{
		Type:           config.SBITypeNetconf,
		DeviceProfile:  config.DeviceProfileCiscoIOSXR,
		NetconfOptions: &config.SBINetconfOptions{},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Netconf == nil {
		t.Errorf("BuildPlan: expected Netconf plan, got nil")
	}
	if plan.Gnmi != nil {
		t.Errorf("BuildPlan: expected no Gnmi plan, got non-nil")
	}
}

// TestBuildPlan_NonSonic_NotAffectedBySonicBranch checks that a generic gNMI
// profile still gets the standard single-root plan after the sonic branch lands.
func TestBuildPlan_NonSonic_NotAffectedBySonicBranch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)

	addAndFinish(t, root, interfaceUpdates("ethernet-1/1", "uplink"), testhelper.FlagsNew)

	sbi := &config.SBI{
		Type:        config.SBITypeGnmi,
		GnmiOptions: &config.SBIGnmiOptions{Encoding: "PROTO"},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Gnmi == nil {
		t.Fatalf("BuildPlan: expected Gnmi plan, got nil")
	}
	for _, u := range plan.Gnmi.Updates {
		if u.GetPath().GetOrigin() != "" {
			t.Errorf("generic plan must not set Path.Origin, got %q", u.GetPath().GetOrigin())
		}
	}
}

// --- Existing generic path tests -----------------------------------------

func TestBuildPlan_GnmiSBI_ReturnsGnmiSetPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	sbi := &config.SBI{
		Type:        config.SBITypeGnmi,
		GnmiOptions: &config.SBIGnmiOptions{Encoding: "PROTO"},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Gnmi == nil {
		t.Errorf("BuildPlan: expected Gnmi plan, got nil")
	}
	if plan.Netconf != nil {
		t.Errorf("BuildPlan: expected no Netconf plan, got non-nil")
	}
}

func TestBuildPlan_NetconfSBI_ReturnsNetconfSetPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	sbi := &config.SBI{
		Type:           "netconf",
		NetconfOptions: &config.SBINetconfOptions{},
	}

	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Netconf == nil {
		t.Errorf("BuildPlan: expected Netconf plan, got nil")
	}
	if plan.Gnmi != nil {
		t.Errorf("BuildPlan: expected no Gnmi plan, got non-nil")
	}
}

// TestBuildPlan_Netconf_NoChanges_StillReturnsNonNilPlan pins down an
// invariant that buildNetconfPlan currently satisfies "for free" (it always
// wraps a *NetconfSetPlan, even for an empty document): unlike the gNMI side
// (see sonic.Encode / NewGnmiPlan), no NETCONF encoder today returns nil to
// signal "nothing to apply". If a future NETCONF device-profile encoder
// adopts that convention, it must be normalized the same way NewGnmiPlan
// normalizes a nil GnmiSetPlan — otherwise it reproduces this bug class on
// the NETCONF side.
func TestBuildPlan_Netconf_NoChanges_StillReturnsNonNilPlan(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	root, scb := newTestRoot(t, mockCtrl)
	sbi := &config.SBI{
		Type:           "netconf",
		NetconfOptions: &config.SBINetconfOptions{},
	}

	// root.Entry has no updates applied: this is the no-op case.
	plan, err := materialize.BuildPlan(context.Background(), scb, sbi, root.Entry, false)
	if err != nil {
		t.Fatalf("BuildPlan: unexpected error: %v", err)
	}
	if plan.Netconf == nil {
		t.Fatalf("BuildPlan: expected non-nil Netconf plan even for a no-op transaction, got nil")
	}
	if plan.Gnmi != nil {
		t.Errorf("BuildPlan: expected no Gnmi plan, got non-nil")
	}
}
