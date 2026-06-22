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

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestServer builds the minimal Server needed to call CreateDataStore.
// It wires a real in-memory schema client and a mock cache client that
// reports the cache instance as already existing (so no blocking InstanceCreate
// loop is triggered).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	ctrl := gomock.NewController(t)

	sc, _, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("InitSDCIOSchema: %v", err)
	}

	mockCC := mockcacheclient.NewMockClient(ctrl)
	// Report the cache instance as already existing so initCache returns immediately.
	mockCC.EXPECT().
		InstanceExists(gomock.Any(), gomock.Any()).
		Return(true).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return &Server{
		datastores:   NewDatastoreMap(),
		schemaClient: sc,
		cacheClient:  mockCC,
		ctx:          ctx,
		config: &config.Config{
			Validation: config.NewValidationConfig(),
			Deviation:  &config.DeviationConfig{},
		},
	}
}

// noopCreateReq builds a minimal CreateDataStoreRequest with the given SBI type.
func noopCreateReq(dsName, sbiType string) *sdcpb.CreateDataStoreRequest {
	return &sdcpb.CreateDataStoreRequest{
		DatastoreName: dsName,
		Target: &sdcpb.Target{
			Type: sbiType,
		},
		// Empty but non-nil Sync prevents a nil-pointer in the connectSBI goroutine.
		Sync: &sdcpb.Sync{},
		Schema: &sdcpb.Schema{
			Name:    "testschema",
			Vendor:  "sdcio",
			Version: "v0.0.0",
		},
	}
}

// TestCreateDataStore_RejectsUnknownType verifies that an unrecognised SBI type
// returns an error whose message contains "unknown targetconnection protocol type"
// (no typo).
func TestCreateDataStore_RejectsUnknownType(t *testing.T) {
	s := &Server{
		datastores: NewDatastoreMap(),
		config: &config.Config{
			Validation: config.NewValidationConfig(),
			Deviation:  &config.DeviationConfig{},
		},
	}

	_, err := s.CreateDataStore(context.Background(), noopCreateReq("ds1", "notaprotocol"))
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
	const want = "unknown targetconnection protocol type"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

// TestCreateDataStore_AcceptsNoopType verifies that sbi.type = "noop" creates the
// datastore without error.
func TestCreateDataStore_AcceptsNoopType(t *testing.T) {
	s := newTestServer(t)

	_, err := s.CreateDataStore(context.Background(), noopCreateReq("ds-noop", "noop"))
	if err != nil {
		t.Fatalf("expected no error for noop type, got: %v", err)
	}
}

// TestCreateDataStore_NoopType_WithFullConnectionProfile verifies that supplying a
// fully-populated connection profile (address, port, TLS, credentials) alongside
// sbi.type = "noop" does not cause an error.
func TestCreateDataStore_NoopType_WithFullConnectionProfile(t *testing.T) {
	s := newTestServer(t)

	req := noopCreateReq("ds-noop-full", "noop")
	req.Target.Address = "192.0.2.1"
	req.Target.Port = 830
	req.Target.Tls = &sdcpb.TLS{
		Ca:         "ca-cert",
		Cert:       "client-cert",
		Key:        "client-key",
		SkipVerify: true,
	}
	req.Target.Credentials = &sdcpb.Credentials{
		Username: "admin",
		Password: "secret",
	}

	_, err := s.CreateDataStore(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error for noop type with full profile, got: %v", err)
	}
}

// TestCreateDataStore_EmptyType_ReturnsInvalidArgument verifies that omitting
// sbi.type (empty string) returns a gRPC codes.InvalidArgument error rather
// than silently creating a noop datastore.
func TestCreateDataStore_EmptyType_ReturnsInvalidArgument(t *testing.T) {
	s := &Server{
		datastores: NewDatastoreMap(),
		config: &config.Config{
			Validation: config.NewValidationConfig(),
			Deviation:  &config.DeviationConfig{},
		},
	}

	_, err := s.CreateDataStore(context.Background(), noopCreateReq("ds-empty", ""))
	if err == nil {
		t.Fatal("expected error for empty SBI type, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("status code = %v, want %v", st.Code(), codes.InvalidArgument)
	}
}

// TestCreateDataStore_NoopType_WithSyncConfig_Succeeds verifies that a noop
// datastore accepts a populated sync config without error.  The noop target
// silently discards all sync entries, so they must not cause a rejection.
func TestCreateDataStore_NoopType_WithSyncConfig_Succeeds(t *testing.T) {
	s := newTestServer(t)

	req := noopCreateReq("ds-noop-sync", "noop")
	req.Sync = &sdcpb.Sync{
		Config: []*sdcpb.SyncConfig{
			{
				Name:   "sync-all",
				Target: &sdcpb.Target{Type: "gnmi"},
				Path:   []string{"/"},
				Mode:   sdcpb.SyncMode_SM_ON_CHANGE,
			},
		},
	}

	_, err := s.CreateDataStore(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error for noop type with sync config, got: %v", err)
	}
}
