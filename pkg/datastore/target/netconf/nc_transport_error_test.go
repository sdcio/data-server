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
	"io"
	"sync"
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
			CommitDatastore: "candidate",
		},
	}
}

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
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().GetConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	d.EXPECT().Close().Return(nil)

	tr := &ncTarget{
		name:      "dev1",
		m:         new(sync.Mutex),
		driver:    d,
		sbiConfig: testSBIConfig(),
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
		m:         new(sync.Mutex),
		driver:    d,
		sbiConfig: testSBIConfig(),
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
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(nil, io.EOF)
	d.EXPECT().Close().Return(nil)
	// Discard must NOT be called for a transport error.

	tr := &ncTarget{
		name:      "dev1",
		m:         new(sync.Mutex),
		driver:    d,
		sbiConfig: testSBIConfig(),
	}

	_, err := tr.setToDevice(context.Background(), "candidate", newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected errors.Is(err, types.ErrNotConnected), got %v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected the original io.EOF to still be discoverable via errors.Is, got %v", err)
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
		m:         new(sync.Mutex),
		driver:    d,
		sbiConfig: testSBIConfig(),
	}

	_, err := tr.setToDevice(context.Background(), "candidate", newFakeTargetSource())
	if !errors.Is(err, nonTransportErr) {
		t.Errorf("expected the original error to be returned unwrapped, got %v", err)
	}
	if errors.Is(err, types.ErrNotConnected) {
		t.Errorf("did not expect errors.Is(err, types.ErrNotConnected), got %v", err)
	}
}

func Test_ncTarget_setToDevice_Commit_TransportError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	d := mocknetconf.NewMockDriver(mockCtrl)
	d.EXPECT().IsAlive().AnyTimes().Return(true)
	d.EXPECT().EditConfig(gomock.Any(), gomock.Any()).Return(&nctypes.NetconfResponse{Doc: rpcReplyDoc(t)}, nil)
	d.EXPECT().Commit().Return(io.EOF)
	d.EXPECT().Close().Return(nil)

	tr := &ncTarget{
		name:      "dev1",
		m:         new(sync.Mutex),
		driver:    d,
		sbiConfig: testSBIConfig(),
	}

	_, err := tr.setToDevice(context.Background(), "candidate", newFakeTargetSource())
	if !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected errors.Is(err, types.ErrNotConnected), got %v", err)
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected the original io.EOF to still be discoverable via errors.Is, got %v", err)
	}
}