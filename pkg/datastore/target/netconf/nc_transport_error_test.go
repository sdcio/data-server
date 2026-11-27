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
	"strings"
	"sync"
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
	return replyDoc(t, "<rpc-reply></rpc-reply>")
}

func replyDoc(t *testing.T, xml string) *etree.Document {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xml); err != nil {
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
	d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })
	// Discard and Unlock must NOT be called for a transport error.

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
	gomock.InOrder(
		d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil),
		d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, nonTransportErr),
		d.EXPECT().Discard().Return(nil),
		d.EXPECT().Unlock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil),
	)
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
	d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().Commit().Return(nil, io.EOF)
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

func Test_ncTarget_setToDevice_Commit_NonTransportError_DiscardsCandidate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	commitErr := errors.New("rpc-error: commit validation failed")
	gomock.InOrder(
		d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil),
		d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil),
		d.EXPECT().Commit().Return(nil, commitErr),
		d.EXPECT().Discard().Return(nil),
		d.EXPECT().Unlock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil),
	)
	// Close must NOT be called for a non-transport error.

	tr := &ncTarget{
		name:      "dev1",
		driver:    d,
		sbiConfig: testSBIConfig(),
		newDriver: failingDriverFactory,
	}

	_, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if !errors.Is(err, commitErr) {
		t.Errorf("expected the commit error to be returned, got %v", err)
	}
	if errors.Is(err, types.ErrNotConnected) {
		t.Errorf("did not expect errors.Is(err, types.ErrNotConnected), got %v", err)
	}
}

func Test_ncTarget_setToDevice_ReturnsEditAndCommitWarnings(t *testing.T) {
	const warning = `<rpc-error><error-severity>warning</error-severity><error-message>%s</error-message></rpc-error>`
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{
		Doc: replyDoc(t, "<rpc-reply>"+fmt.Sprintf(warning, "edit")+"</rpc-reply>"),
	}, nil)
	d.EXPECT().Commit().Return(&nctypes.NetconfResponse{
		Doc: replyDoc(t, "<rpc-reply>"+fmt.Sprintf(warning, "commit")+"</rpc-reply>"),
	}, nil)
	d.EXPECT().Unlock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)

	tr := &ncTarget{name: "dev1", driver: d, sbiConfig: testSBIConfig(), newDriver: failingDriverFactory}

	rsp, err := tr.setToDevice(context.Background(), config.NCCommitDatastoreCandidate, newFakeTargetSource())
	if err != nil {
		t.Fatalf("setToDevice: %v", err)
	}
	if len(rsp.Warnings) != 2 || !strings.Contains(rsp.Warnings[0], "edit") || !strings.Contains(rsp.Warnings[1], "commit") {
		t.Errorf("expected edit and commit warnings, got %q", rsp.Warnings)
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
	d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, capsChangedErr())
	d.EXPECT().Close().DoAndReturn(func() error { closed <- struct{}{}; return nil })
	// Discard and Unlock must NOT be called when retiring a session due to capabilities-changed.

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
	d.EXPECT().Lock(config.NCCommitDatastoreCandidate).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().Commit().Return(nil, capsChangedErr())
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
	closed := make(chan struct{})
	d.EXPECT().Close().DoAndReturn(func() error { close(closed); return nil })

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

	// the stale driver is closed in the background; wait so gomock sees the call
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("failed driver was not closed")
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

type lockOwner int

const (
	lockNone         lockOwner = 0
	lockThisSession  lockOwner = 1
	lockOtherSession lockOwner = 42
)

// fakeCandidateDriver models an RFC 6241 device with shared <candidate> and
// <running> datastores, session lock ownership, and auto-discard when a locked
// session closes.
type fakeCandidateDriver struct {
	mu             sync.Mutex
	alive          bool
	closedCh       chan struct{}
	closeOnce      sync.Once
	running        string
	candidate      string
	candidateDirty bool
	lockedBy       lockOwner

	editErr      error
	commitErr    error
	discardErr   error
	unlockErr    error
	lockErr      error
	retryLockErr error
}

func newFakeCandidateDriver() *fakeCandidateDriver {
	const initial = "<initial/>"
	return &fakeCandidateDriver{
		alive:     true,
		closedCh:  make(chan struct{}),
		running:   initial,
		candidate: initial,
	}
}

func (f *fakeCandidateDriver) okReply() *nctypes.NetconfResponse {
	doc := etree.NewDocument()
	_ = doc.ReadFromString("<rpc-reply><ok/></rpc-reply>")
	return &nctypes.NetconfResponse{Doc: doc}
}

func (f *fakeCandidateDriver) Get(string) (*nctypes.NetconfResponse, error) {
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) GetConfig(string, string) (*nctypes.NetconfResponse, error) {
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) Validate(string) (*nctypes.NetconfResponse, error) {
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) IsAlive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive
}

func (f *fakeCandidateDriver) Close() error {
	f.mu.Lock()
	f.alive = false
	// Per RFC 6241 §8.3.4, uncommitted changes made under a session lock are
	// discarded when the session holding the lock terminates.
	if f.lockedBy == lockThisSession {
		f.candidate = f.running
		f.candidateDirty = false
		f.lockedBy = lockNone
	}
	f.mu.Unlock()
	f.closeOnce.Do(func() { close(f.closedCh) })
	return nil
}

func (f *fakeCandidateDriver) Lock(target string) (*nctypes.NetconfResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lockErr != nil {
		err := f.lockErr
		f.lockErr = nil
		return nil, err
	}
	if target != config.NCCommitDatastoreCandidate {
		return f.okReply(), nil
	}
	if f.lockedBy != lockNone {
		return nil, fmt.Errorf("%w: locked by session %d", nctypes.ErrLockHeld, f.lockedBy)
	}
	if f.candidateDirty {
		if f.retryLockErr != nil {
			// arm retryLockErr for the retry attempt that follows Discard()
			f.lockErr = f.retryLockErr
			f.retryLockErr = nil
		}
		return nil, errors.New("rpc-error: in-use: candidate has uncommitted changes")
	}
	f.lockedBy = lockThisSession
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) Unlock(target string) (*nctypes.NetconfResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unlockErr != nil {
		err := f.unlockErr
		f.unlockErr = nil
		return nil, err
	}
	if target == config.NCCommitDatastoreCandidate {
		if f.lockedBy != lockThisSession {
			return nil, errors.New("rpc-error: operation-failed: lock not held")
		}
		if f.candidateDirty {
			f.candidate = f.running
			f.candidateDirty = false
		}
		f.lockedBy = lockNone
	}
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) EditConfig(target, cfg string) (*nctypes.NetconfResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if target == config.NCCommitDatastoreRunning {
		if f.editErr != nil {
			return nil, f.editErr
		}
		f.running = cfg
		return f.okReply(), nil
	}
	// stop-on-error may partially modify candidate before returning an error
	f.candidate = cfg
	f.candidateDirty = true
	if f.editErr != nil {
		return nil, f.editErr
	}
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) Commit() (*nctypes.NetconfResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	f.running = f.candidate
	f.candidateDirty = false
	return f.okReply(), nil
}

func (f *fakeCandidateDriver) Discard() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.discardErr != nil {
		err := f.discardErr
		f.discardErr = nil
		return err
	}
	if f.lockedBy != lockNone && f.lockedBy != lockThisSession {
		return errors.New("rpc-error: in-use: candidate locked by another session")
	}
	f.candidate = f.running
	f.candidateDirty = false
	return nil
}

func Test_Set_CandidateLocksAndUnlocks(t *testing.T) {
	t.Run("mid-flight transport drop releases lock and leaves candidate clean via session close", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.editErr = io.EOF

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			lifeCtx:   ctx,
			lifeStop:  stop,
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(ctx, newFakeTargetSource())
		if !errors.Is(err, types.ErrNotConnected) {
			t.Fatalf("Set error = %v, want ErrNotConnected", err)
		}
		<-dev.closedCh

		if dev.candidateDirty {
			t.Error("candidate remained dirty after mid-flight transport drop; expected Lock before EditConfig so session close discards candidate")
		}
		if dev.lockedBy != lockNone {
			t.Errorf("lockedBy = %d, want 0", dev.lockedBy)
		}
	})

	t.Run("back-to-back candidate Sets succeed and release lock after commit", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			newDriver: failingDriverFactory,
		}

		if _, err := tr.Set(context.Background(), newFakeTargetSource()); err != nil {
			t.Fatalf("first Set: %v", err)
		}
		if dev.lockedBy != lockNone {
			t.Fatalf("candidate remained locked after first Set (lockedBy=%d)", dev.lockedBy)
		}
		if _, err := tr.Set(context.Background(), newFakeTargetSource()); err != nil {
			t.Fatalf("second Set: %v", err)
		}
	})

	t.Run("running datastore mode does not lock candidate", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.lockedBy = lockOtherSession
		cfg := testSBIConfig()
		cfg.NetconfOptions.CommitDatastore = config.NCCommitDatastoreRunning

		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: cfg,
			newDriver: failingDriverFactory,
		}

		if _, err := tr.Set(context.Background(), newFakeTargetSource()); err != nil {
			t.Fatalf("Set to running failed while candidate was locked: %v", err)
		}
		if dev.lockedBy != lockOtherSession {
			t.Errorf("running mode touched candidate lock: lockedBy = %d, want %d", dev.lockedBy, lockOtherSession)
		}
	})
}

func Test_Set_DirtyCandidateRecoveryAndLockHeld(t *testing.T) {
	t.Run("dirty unlocked candidate is discarded once and Set succeeds", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.candidate = "<stale-uncommitted/>"
		dev.candidateDirty = true

		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			newDriver: failingDriverFactory,
		}

		if _, err := tr.Set(context.Background(), newFakeTargetSource()); err != nil {
			t.Fatalf("Set on dirty unlocked candidate failed: %v", err)
		}
		if dev.candidateDirty {
			t.Error("candidate remained dirty after Set")
		}
		if dev.lockedBy != lockNone {
			t.Errorf("lockedBy = %d, want 0", dev.lockedBy)
		}
		if !strings.Contains(dev.running, "eth0") {
			t.Errorf("running = %q, want committed interface eth0", dev.running)
		}
	})

	t.Run("lock held by another session fails without wiping candidate", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.lockedBy = lockOtherSession
		dev.candidate = "<operator-uncommitted/>"
		dev.candidateDirty = true

		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(context.Background(), newFakeTargetSource())
		if !errors.Is(err, nctypes.ErrLockHeld) {
			t.Fatalf("Set error = %v, want ErrLockHeld", err)
		}
		if dev.candidate != "<operator-uncommitted/>" {
			t.Errorf("candidate = %q, want operator changes preserved", dev.candidate)
		}
		if !tr.Status().IsConnected() {
			t.Error("session must stay connected after lock-denied")
		}
	})

	t.Run("transport or capabilities-changed error on initial Lock skips Discard and retires session", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			lockErr error
		}{
			{"transport error", io.EOF},
			{"capabilities-changed", capsChangedErr()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dev := newFakeCandidateDriver()
				dev.candidate = "<uncommitted/>"
				dev.candidateDirty = true
				dev.lockErr = tc.lockErr

				ctx, stop := context.WithCancel(context.Background())
				defer stop()
				tr := &ncTarget{
					name:      "dev1",
					driver:    dev,
					sbiConfig: testSBIConfig(),
					lifeCtx:   ctx,
					lifeStop:  stop,
					newDriver: failingDriverFactory,
				}

				_, err := tr.Set(ctx, newFakeTargetSource())
				if !errors.Is(err, types.ErrNotConnected) {
					t.Fatalf("Set error = %v, want ErrNotConnected", err)
				}
				if tr.Status().IsConnected() {
					t.Error("expected Status() to be disconnected after session error on Lock")
				}
				<-dev.closedCh
				if dev.candidate != "<uncommitted/>" {
					t.Errorf("candidate = %q, want untouched when Lock failed with session error", dev.candidate)
				}
			})
		}
	})

	t.Run("retry Lock failure stops after single Discard", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.candidate = "<uncommitted/>"
		dev.candidateDirty = true
		retryErr := errors.New("rpc-error: lock-denied on retry")
		dev.retryLockErr = retryErr

		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(context.Background(), newFakeTargetSource())
		if !errors.Is(err, retryErr) {
			t.Fatalf("Set error = %v, want %v", err, retryErr)
		}
		if !tr.Status().IsConnected() {
			t.Error("session must stay connected when retry Lock fails with non-transport error")
		}
	})

	t.Run("failed pre-lock discard with non-transport error preserves healthy session", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.candidate = "<uncommitted/>"
		dev.candidateDirty = true
		dev.discardErr = errors.New("rpc-error: in-use")

		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(context.Background(), newFakeTargetSource())
		if err == nil {
			t.Fatal("expected Set error when both Lock and pre-lock Discard fail")
		}
		if errors.Is(err, types.ErrNotConnected) {
			t.Errorf("did not expect ErrNotConnected, got %v", err)
		}
		if !tr.Status().IsConnected() {
			t.Error("session must stay connected when pre-lock Discard fails with non-transport error")
		}
	})

	t.Run("transport error during pre-lock discard retires session", func(t *testing.T) {
		dev := newFakeCandidateDriver()
		dev.candidate = "<uncommitted/>"
		dev.candidateDirty = true
		dev.discardErr = io.EOF

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:      "dev1",
			driver:    dev,
			sbiConfig: testSBIConfig(),
			lifeCtx:   ctx,
			lifeStop:  stop,
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(ctx, newFakeTargetSource())
		if !errors.Is(err, types.ErrNotConnected) {
			t.Fatalf("Set error = %v, want ErrNotConnected", err)
		}
		if tr.Status().IsConnected() {
			t.Error("expected Status() to be disconnected")
		}
		<-dev.closedCh
	})
}

func Test_Set_RetiresPoisonedSessionOnFailedDiscardOrUnlock(t *testing.T) {
	t.Run("failed post-commit Discard retires session, reconnects to fresh session, and preserves commit error", func(t *testing.T) {
		poisoned := newFakeCandidateDriver()
		commitErr := errors.New("rpc-error: must constraint violated")
		poisoned.commitErr = commitErr
		poisoned.discardErr = errors.New("rpc-error: discard failed")

		fresh := newFakeCandidateDriver()

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:      "dev1",
			driver:    poisoned,
			sbiConfig: testSBIConfig(),
			lifeCtx:   ctx,
			lifeStop:  stop,
			newDriver: func(*config.SBI) (Driver, error) { return fresh, nil },
		}

		_, err := tr.Set(ctx, newFakeTargetSource())
		if !errors.Is(err, commitErr) {
			t.Fatalf("Set error = %v, want %v", err, commitErr)
		}
		if errors.Is(err, types.ErrNotConnected) {
			t.Errorf("semantic commit error must not be wrapped in ErrNotConnected, got %v", err)
		}

		select {
		case <-poisoned.closedCh:
		case <-time.After(time.Second):
			t.Fatal("poisoned session was not closed after failed Discard")
		}
		if poisoned.candidateDirty || poisoned.lockedBy != lockNone {
			t.Errorf("closing poisoned session must release lock and dirty candidate (dirty=%v, lockedBy=%d)", poisoned.candidateDirty, poisoned.lockedBy)
		}

		deadline := time.Now().Add(2 * time.Second)
		for !tr.Status().IsConnected() {
			if time.Now().After(deadline) {
				t.Fatal("target did not reconnect to fresh session")
			}
			time.Sleep(time.Millisecond)
		}
		if _, err := tr.Set(ctx, newFakeTargetSource()); err != nil {
			t.Fatalf("Set on reconnected fresh session failed: %v", err)
		}
		if !strings.Contains(fresh.running, "eth0") {
			t.Errorf("fresh.running = %q, want committed eth0", fresh.running)
		}
	})

	t.Run("failed Unlock after failed EditConfig retires session and preserves edit error", func(t *testing.T) {
		poisoned := newFakeCandidateDriver()
		editErr := errors.New("rpc-error: unknown-element")
		poisoned.editErr = editErr
		poisoned.unlockErr = errors.New("rpc-error: unlock failed")

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:      "dev1",
			driver:    poisoned,
			sbiConfig: testSBIConfig(),
			lifeCtx:   ctx,
			lifeStop:  stop,
			newDriver: failingDriverFactory,
		}

		_, err := tr.Set(ctx, newFakeTargetSource())
		if !errors.Is(err, editErr) {
			t.Fatalf("Set error = %v, want %v", err, editErr)
		}
		if errors.Is(err, types.ErrNotConnected) {
			t.Errorf("semantic edit error must not be wrapped in ErrNotConnected, got %v", err)
		}
		if tr.Status().IsConnected() {
			t.Error("expected Status() to be disconnected after retiring poisoned session")
		}

		select {
		case <-poisoned.closedCh:
		case <-time.After(time.Second):
			t.Fatal("session with stuck lock was not closed after failed Unlock")
		}
		if poisoned.lockedBy != lockNone {
			t.Errorf("lockedBy = %d, want 0", poisoned.lockedBy)
		}
	})

	t.Run("failed Unlock after successful Commit returns success and retires session to release lock", func(t *testing.T) {
		poisoned := newFakeCandidateDriver()
		poisoned.unlockErr = io.EOF

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:      "dev1",
			driver:    poisoned,
			sbiConfig: testSBIConfig(),
			lifeCtx:   ctx,
			lifeStop:  stop,
			newDriver: failingDriverFactory,
		}

		rsp, err := tr.Set(ctx, newFakeTargetSource())
		if err != nil {
			t.Fatalf("Set failed after Commit already succeeded: %v", err)
		}
		if rsp == nil {
			t.Fatal("expected non-nil SetDataResponse")
		}
		if !strings.Contains(poisoned.running, "eth0") {
			t.Errorf("running = %q, want committed eth0", poisoned.running)
		}
		if tr.Status().IsConnected() {
			t.Error("expected Status() to be disconnected after retiring session with failed Unlock")
		}

		select {
		case <-poisoned.closedCh:
		case <-time.After(time.Second):
			t.Fatal("session with stuck lock was not closed after post-Commit Unlock failure")
		}
		if poisoned.lockedBy != lockNone {
			t.Errorf("lockedBy = %d, want 0", poisoned.lockedBy)
		}
	})

	t.Run("capabilities-changed on post-edit Discard honors cooldown while preserving edit error", func(t *testing.T) {
		poisoned := newFakeCandidateDriver()
		editErr := errors.New("rpc-error: bad-element")
		poisoned.editErr = editErr
		poisoned.discardErr = capsChangedErr()

		const cooldown = 100 * time.Millisecond
		var dialed atomic.Bool
		fresh := newFakeCandidateDriver()

		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		tr := &ncTarget{
			name:                  "dev1",
			driver:                poisoned,
			sbiConfig:             testSBIConfig(),
			lifeCtx:               ctx,
			lifeStop:              stop,
			capsReconnectCooldown: cooldown,
			nextCapsReconnect:     time.Now().Add(cooldown),
			newDriver: func(*config.SBI) (Driver, error) {
				dialed.Store(true)
				return fresh, nil
			},
		}

		start := time.Now()
		_, err := tr.Set(ctx, newFakeTargetSource())
		if !errors.Is(err, editErr) || errors.Is(err, types.ErrNotConnected) {
			t.Fatalf("Set error = %v, want unwrapped %v", err, editErr)
		}
		if tr.Status().IsConnected() {
			t.Error("expected Status() to be disconnected immediately")
		}
		if time.Since(start) < cooldown/2 && dialed.Load() {
			t.Error("reconnect dialed before capabilities-changed cooldown elapsed")
		}
		<-poisoned.closedCh

		deadline := time.Now().Add(2 * time.Second)
		for !tr.Status().IsConnected() {
			if time.Now().After(deadline) {
				t.Fatal("target did not reconnect after cooldown")
			}
			time.Sleep(time.Millisecond)
		}
	})
}
