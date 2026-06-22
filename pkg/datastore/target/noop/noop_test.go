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

package noop_test

import (
	"context"
	"testing"

	"github.com/beevik/etree"
	"github.com/go-logr/logr"
	sdclogger "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/noop"
)

// stubSource is a minimal TargetSource that returns pre-configured updates and
// deletes. All other methods are stubs that return zero values.
type stubSource struct {
	updates []*sdcpb.Update
	deletes []*sdcpb.Path
}

func (s *stubSource) ToProtoUpdates(_ context.Context, _ bool) ([]*sdcpb.Update, error) {
	return s.updates, nil
}
func (s *stubSource) ToProtoDeletes(_ context.Context) ([]*sdcpb.Path, error) {
	return s.deletes, nil
}
func (s *stubSource) ToJson(_ context.Context, _ bool) (any, error)    { return nil, nil }
func (s *stubSource) ToJsonIETF(_ context.Context, _ bool) (any, error) { return nil, nil }
func (s *stubSource) ToXML(_ context.Context, _ bool, _, _, _ bool) (*etree.Document, error) {
	return nil, nil
}
func (s *stubSource) ContainsChanges(_ context.Context) (bool, error) { return false, nil }

// infoCapture records every Info-level (V(0)) log call.
type infoCapture struct {
	calls []string
}

func (c *infoCapture) Init(logr.RuntimeInfo)                                      {}
func (c *infoCapture) Enabled(level int) bool                                     { return true }
func (c *infoCapture) Info(level int, msg string, _ ...interface{})               {
	if level == 0 {
		c.calls = append(c.calls, msg)
	}
}
func (c *infoCapture) Error(_ error, _ string, _ ...interface{}) {}
func (c *infoCapture) WithValues(_ ...interface{}) logr.LogSink  { return c }
func (c *infoCapture) WithName(_ string) logr.LogSink            { return c }

func ctxWithCapture(cap *infoCapture) context.Context {
	log := logr.New(cap)
	return sdclogger.IntoContext(context.Background(), log)
}

// TestAddSyncs_ZeroEntries_ReturnsNil verifies that AddSyncs called with no
// entries returns nil.
func TestAddSyncs_ZeroEntries_ReturnsNil(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	if err := tgt.AddSyncs(context.Background()); err != nil {
		t.Fatalf("AddSyncs() error = %v, want nil", err)
	}
}

// TestAddSyncs_MultipleEntries_ReturnsNil verifies that AddSyncs called with
// one or more SyncProtocol entries always returns nil.
func TestAddSyncs_MultipleEntries_ReturnsNil(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	syncs := []*config.SyncProtocol{
		{Name: "s1", Protocol: "gnmi"},
		{Name: "s2", Protocol: "netconf"},
	}
	if err := tgt.AddSyncs(context.Background(), syncs...); err != nil {
		t.Fatalf("AddSyncs(%d entries) error = %v, want nil", len(syncs), err)
	}
}

// TestAddSyncs_NoInfoLogPerEntry verifies that AddSyncs does NOT emit an
// Info-level log line for each sync entry it receives.  The current
// implementation logs each marshalled entry at Info, which is noisy in CI and
// implies meaningful processing; the fixed implementation silently discards
// them.
func TestAddSyncs_NoInfoLogPerEntry(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	cap := &infoCapture{}
	ctx := ctxWithCapture(cap)

	syncs := []*config.SyncProtocol{
		{Name: "s1", Protocol: "gnmi"},
		{Name: "s2", Protocol: "netconf"},
	}
	if err := tgt.AddSyncs(ctx, syncs...); err != nil {
		t.Fatalf("AddSyncs: %v", err)
	}

	if len(cap.calls) > 0 {
		t.Errorf("AddSyncs emitted %d Info-level log(s) %v; want zero", len(cap.calls), cap.calls)
	}
}

// TestStatus_ReturnsConnected verifies that a freshly-created noop target
// reports itself as connected.
func TestStatus_ReturnsConnected(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	st := tgt.Status()
	if st == nil {
		t.Fatal("Status() returned nil")
	}
	if !st.IsConnected() {
		t.Errorf("Status().IsConnected() = false, want true")
	}
}

// TestClose_ReturnsNil verifies that Close always returns nil.
func TestClose_ReturnsNil(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	if err := tgt.Close(context.Background()); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// TestGet_ReturnsOneNotificationPerPath verifies that Get returns exactly one
// Notification for each requested path, and that each notification carries a
// non-zero timestamp.
func TestGet_ReturnsOneNotificationPerPath(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	paths := []*sdcpb.Path{
		{Elem: []*sdcpb.PathElem{{Name: "interface"}, {Name: "ethernet-1/1"}}},
		{Elem: []*sdcpb.PathElem{{Name: "network-instance"}, {Name: "default"}}},
	}
	req := &sdcpb.GetDataRequest{Path: paths}

	resp, err := tgt.Get(context.Background(), req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got, want := len(resp.Notification), len(paths); got != want {
		t.Fatalf("len(Notification) = %d, want %d", got, want)
	}
	for i, n := range resp.Notification {
		if n.Timestamp == 0 {
			t.Errorf("Notification[%d].Timestamp = 0, want non-zero", i)
		}
	}
}

// TestSet_ReturnsCorrectUpdateResultOps verifies that Set returns one
// UpdateResult per update (Op=UPDATE) and one per delete (Op=DELETE), in that
// order.
func TestSet_ReturnsCorrectUpdateResultOps(t *testing.T) {
	tgt, err := noop.NewNoopTarget(context.Background(), "ds-noop")
	if err != nil {
		t.Fatalf("NewNoopTarget: %v", err)
	}

	updPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}}
	delPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "network-instance"}}}

	src := &stubSource{
		updates: []*sdcpb.Update{{Path: updPath, Value: &sdcpb.TypedValue{}}},
		deletes: []*sdcpb.Path{delPath},
	}

	resp, err := tgt.Set(context.Background(), src)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got, want := len(resp.Response), 2; got != want {
		t.Fatalf("len(Response) = %d, want %d", got, want)
	}
	if resp.Response[0].Op != sdcpb.UpdateResult_UPDATE {
		t.Errorf("Response[0].Op = %v, want UPDATE", resp.Response[0].Op)
	}
	if resp.Response[1].Op != sdcpb.UpdateResult_DELETE {
		t.Errorf("Response[1].Op = %v, want DELETE", resp.Response[1].Op)
	}
}
