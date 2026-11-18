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

package netconf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/mocks/mocknetconf"
	"github.com/sdcio/data-server/pkg/config"
	nctypes "github.com/sdcio/data-server/pkg/datastore/target/netconf/types"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// fakeTargetSource is a minimal types.TargetSource used to drive setToDevice
// without needing a real datastore tree.
type fakeTargetSource struct {
	doc *etree.Document
}

func newFakeTargetSource() *fakeTargetSource {
	doc := etree.NewDocument()
	if err := doc.ReadFromString("<interface><name>eth0</name></interface>"); err != nil {
		panic(err)
	}
	return &fakeTargetSource{doc: doc}
}

func (f *fakeTargetSource) ToJson(_ context.Context, _ bool) (any, error) { return nil, nil }
func (f *fakeTargetSource) ToJsonIETF(_ context.Context, _ bool) (any, error) {
	return nil, nil
}

func (f *fakeTargetSource) ToXML(_ context.Context, _, _, _, _ bool) (*etree.Document, error) {
	return f.doc, nil
}

func (f *fakeTargetSource) ToProtoUpdates(_ context.Context, _ bool) ([]*sdcpb.Update, error) {
	return nil, nil
}

func (f *fakeTargetSource) ToProtoDeletes(_ context.Context) ([]*sdcpb.Path, error) {
	return nil, nil
}

func (f *fakeTargetSource) ContainsChanges(_ context.Context) (bool, error) {
	return true, nil
}

// testSBIConfig returns a minimal SBI config good enough to let a background
// reconnect() attempt (and quickly fail/retry against a bogus address)
// without blocking the test itself, since reconnect is always launched via
// `go t.reconnect(ctx)`.
func testSBIConfig() *config.SBI {
	return &config.SBI{
		Address:      "127.0.0.1:0",
		ConnectRetry: time.Millisecond,
		NetconfOptions: &config.SBINetconfOptions{
			CommitDatastore: config.NCCommitDatastoreCandidate,
		},
	}
}

// failingDriverFactory keeps background reconnects from dialing anything.
func failingDriverFactory(*config.SBI) (Driver, error) { return nil, errors.New("no device") }

func rpcReplyDoc(t *testing.T) *etree.Document {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString("<rpc-reply></rpc-reply>"); err != nil {
		t.Fatalf("building rpc-reply doc: %v", err)
	}
	return doc
}

func Test_ncTarget_internalGet_TransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected errors.Is(err, types.ErrNotConnected), got %v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected the original io.EOF to still be discoverable via errors.Is, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("failed driver was not closed")
	}
}

func Test_ncTarget_internalGet_NonTransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	nonTransportErr := errors.New("invalid filter")
	d.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, nonTransportErr)
	// Close must NOT be called for a non-transport error; the strict mock
	// controller will fail the test if it is.

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if !errors.Is(err, nonTransportErr) {
		t.Errorf("expected the original error to be returned unwrapped, got %v", err)
	}
	if errors.Is(err, types.ErrNotConnected) {
		t.Errorf("did not expect errors.Is(err, types.ErrNotConnected), got %v", err)
	}
}

func Test_ncTarget_setToDevice_EditConfig_TransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })
	// Discard must NOT be called for a transport error.

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected errors.Is(err, types.ErrNotConnected), got %v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected the original io.EOF to still be discoverable via errors.Is, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("failed driver was not closed")
	}
}

func Test_ncTarget_setToDevice_EditConfig_NonTransportError_DiscardsCandidate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	nonTransportErr := errors.New("rpc-error: data-exists")
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, nonTransportErr)
	d.EXPECT().Discard().Return(nil)
	// Close must NOT be called for a non-transport error.

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, nonTransportErr) {
		t.Errorf("expected the original error to be returned unwrapped, got %v", err)
	}
	if errors.Is(err, types.ErrNotConnected) {
		t.Errorf("did not expect errors.Is(err, types.ErrNotConnected), got %v", err)
	}
}

func Test_ncTarget_setToDevice_Commit_TransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().Commit().Return(io.EOF)
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected errors.Is(err, types.ErrNotConnected), got %v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected the original io.EOF to still be discoverable via errors.Is, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("failed driver was not closed")
	}
}

func Test_ncTarget_ReconnectsAfterTransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	old := mocknetconf.NewMockDriver(mockCtrl)
	old.EXPECT().IsAlive().AnyTimes().Return(true)
	old.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	closed := make(chan struct{})
	old.EXPECT().Close().DoAndReturn(func() error { close(closed); return nil })

	fresh := mocknetconf.NewMockDriver(mockCtrl)
	fresh.EXPECT().IsAlive().AnyTimes().Return(true)
	fresh.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)

	attempts := 0
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tr := &ncTarget{
		name:      "dev1",
		driver:    old,
		sbiConfig: testSBIConfig(),
		lifeCtx:   ctx,
		lifeStop:  stop,
		newDriver: func(*config.SBI) (Driver, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("device still down")
			}
			return fresh, nil
		},
	}

	_, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if !errors.Is(err, types.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("failed driver was not closed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !tr.Status().IsConnected() {
		if time.Now().After(deadline) {
			t.Fatal("target did not reconnect")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{}); err != nil {
		t.Fatalf("Get after reconnect: %v", err)
	}
}

func Test_ncTarget_StaleFailureDoesNotCloseNewDriver(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	stale := mocknetconf.NewMockDriver(mockCtrl)
	current := mocknetconf.NewMockDriver(mockCtrl)
	current.EXPECT().IsAlive().AnyTimes().Return(true)
	closed := make(chan struct{})
	stale.EXPECT().Close().DoAndReturn(func() error { close(closed); return nil })

	tr := &ncTarget{name: "dev1", driver: current, sbiConfig: testSBIConfig(), newDriver: failingDriverFactory}
	_ = tr.handleTransportError(context.Background(), stale, io.EOF)
	<-closed

	if !tr.Status().IsConnected() {
		t.Error("a failure on a replaced driver must not disconnect the current one")
	}
}

func capsChangedErr() error {
	return fmt.Errorf("%w: rpc-error", nctypes.ErrCapabilitiesChanged)
}

func Test_ncTarget_internalGet_CapabilitiesChanged_Reconnects(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })

	fresh := mocknetconf.NewMockDriver(mockCtrl)
	fresh.EXPECT().IsAlive().AnyTimes().Return(true)
	fresh.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		lifeCtx:   ctx,
		lifeStop:  stop,
		newDriver: func(*config.SBI) (Driver, error) { return fresh, nil },
	}

	_, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if !errors.Is(err, types.ErrNotConnected) || !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Fatalf("expected ErrNotConnected wrapping ErrCapabilitiesChanged, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("failed driver was not closed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !tr.Status().IsConnected() {
		if time.Now().After(deadline) {
			t.Fatal("target did not reconnect")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{}); err != nil {
		t.Fatalf("Get after capabilities-changed reconnect: %v", err)
	}
}

func Test_ncTarget_setToDevice_EditConfig_CapabilitiesChanged_SkipsDiscard(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })
	// Discard must NOT be called when retiring a session due to capabilities-changed.

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tr := &ncTarget{name: "dev1", driver: d, sbiConfig: testSBIConfig(), lifeCtx: ctx, lifeStop: stop, newDriver: failingDriverFactory}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) || !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Fatalf("expected ErrNotConnected wrapping ErrCapabilitiesChanged, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("failed driver was not closed")
	}
}

func Test_ncTarget_setToDevice_Commit_CapabilitiesChanged_Reconnects(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	closed := make(chan struct{}, 1)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().Commit().Return(capsChangedErr())
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tr := &ncTarget{name: "dev1", driver: d, sbiConfig: testSBIConfig(), lifeCtx: ctx, lifeStop: stop, newDriver: failingDriverFactory}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) || !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Fatalf("expected ErrNotConnected wrapping ErrCapabilitiesChanged, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("failed driver was not closed")
	}
}

func Test_ncTarget_CapabilitiesChanged_CooldownDelaysSecondReconnect(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	d1 := mocknetconf.NewMockDriver(mockCtrl)
	d1.EXPECT().IsAlive().AnyTimes().Return(true)
	d1.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d1Closed := make(chan struct{})
	d1.EXPECT().Close().DoAndReturn(func() error { close(d1Closed); return nil })

	d2 := mocknetconf.NewMockDriver(mockCtrl)
	d2.EXPECT().IsAlive().AnyTimes().Return(true)
	d2.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d2Closed := make(chan struct{})
	d2.EXPECT().Close().DoAndReturn(func() error { close(d2Closed); return nil })

	d3 := mocknetconf.NewMockDriver(mockCtrl)
	d3.EXPECT().IsAlive().AnyTimes().Return(true)
	d3.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)

	const cooldown = 120 * time.Millisecond
	var dials atomic.Int32

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tr := &ncTarget{
		name:                  "dev1",
		driver:                d1,
		sbiConfig:             testSBIConfig(),
		lifeCtx:               ctx,
		lifeStop:              stop,
		capsReconnectCooldown: cooldown,
		newDriver: func(*config.SBI) (Driver, error) {
			if dials.Add(1) == 1 {
				return d2, nil
			}
			return d3, nil
		},
	}

	// First capabilities-changed reconnects without delay.
	start := time.Now()
	if _, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{}); !errors.Is(err, types.ErrNotConnected) {
		t.Fatalf("first Get: expected ErrNotConnected, got %v", err)
	}
	<-d1Closed
	deadline := time.Now().Add(time.Second)
	for !tr.Status().IsConnected() {
		if time.Now().After(deadline) {
			t.Fatal("target did not complete first reconnect")
		}
		time.Sleep(time.Millisecond)
	}

	// Second capabilities-changed inside the cooldown immediately retires d2,
	// reports ErrNotConnected, and waits out the remaining cooldown before dialing d3.
	_, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if !errors.Is(err, types.ErrNotConnected) || !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Fatalf("second Get: expected ErrNotConnected wrapping ErrCapabilitiesChanged, got %v", err)
	}
	if tr.Status().IsConnected() {
		t.Fatal("stale second session must be retired immediately")
	}
	<-d2Closed

	if time.Since(start) < cooldown/2 && dials.Load() > 1 {
		t.Fatalf("second reconnect dialed before cooldown elapsed (dials=%d, elapsed=%v)", dials.Load(), time.Since(start))
	}

	deadline = time.Now().Add(2 * time.Second)
	for !tr.Status().IsConnected() {
		if time.Now().After(deadline) {
			t.Fatal("target did not complete delayed second reconnect")
		}
		time.Sleep(time.Millisecond)
	}
	if elapsed := time.Since(start); elapsed < cooldown {
		t.Errorf("second reconnect finished after %v, want >= %v", elapsed, cooldown)
	}
	if _, err := tr.internalGet(context.Background(), &sdcpb.GetDataRequest{}); err != nil {
		t.Fatalf("Get on third session: %v", err)
	}
}

func Test_ncTarget_CapabilitiesChanged_CloseCancelsCooldownWait(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d.EXPECT().Close().Return(nil)

	ctx, stop := context.WithCancel(context.Background())
	tr := &ncTarget{
		name:                  "dev1",
		driver:                d,
		sbiConfig:             testSBIConfig(),
		lifeCtx:               ctx,
		lifeStop:              stop,
		capsReconnectCooldown: 10 * time.Second,
		nextCapsReconnect:     time.Now().Add(10 * time.Second),
		newDriver:             failingDriverFactory,
	}

	_, _ = tr.internalGet(context.Background(), &sdcpb.GetDataRequest{})
	if err := tr.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		tr.dm.RLock()
		busy := tr.reconnecting
		tr.dm.RUnlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not cancel pending cooldown wait")
		}
		time.Sleep(time.Millisecond)
	}
}

func Test_ncTarget_CapabilitiesChanged_StaleDriverDoesNotDisconnectOrAdvanceCooldown(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	stale := mocknetconf.NewMockDriver(mockCtrl)
	closed := make(chan struct{})
	stale.EXPECT().Close().DoAndReturn(func() error { close(closed); return nil })

	current := mocknetconf.NewMockDriver(mockCtrl)
	current.EXPECT().IsAlive().AnyTimes().Return(true)

	tr := &ncTarget{name: "dev1", driver: current, sbiConfig: testSBIConfig(), newDriver: failingDriverFactory}
	err := tr.handleCapabilitiesChanged(context.Background(), stale, capsChangedErr())
	<-closed

	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected for a replaced driver, got %v", err)
	}
	if !tr.Status().IsConnected() {
		t.Error("a failure on a replaced driver must not disconnect the current one")
	}
	if !tr.nextCapsReconnect.IsZero() {
		t.Errorf("stale driver failure must not advance nextCapsReconnect, got %v", tr.nextCapsReconnect)
	}
}
