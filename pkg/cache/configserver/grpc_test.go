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
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/mocks/mockconfigread"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/sdcio/sdc-protos/config_read"
)

func TestGRPCConfigClient_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	client.EXPECT().Get(gomock.Any(), &config_read.GetConfigRequest{
		TargetNamespace: "ns1",
		TargetName:      "target1",
		Name:            "intent1",
	}).Return(&config_read.GetConfigResponse{
		Config: &config_read.ConfigEntry{
			Name:         "intent1",
			Namespace:    "ns1",
			Priority:     10,
			NonRevertive: true,
			Orphan:       true,
			SensitivePaths: []*sdcpb.Path{
				{Elem: []*sdcpb.PathElem{{Name: "secret"}}},
			},
			Config: []*config_read.ConfigBlob{
				{Path: "/interface[name=eth0]/description", Value: []byte(`"uplink"`)},
			},
		},
	}, nil)

	r := NewGRPCConfigClient(nil)
	r.client = client

	want := &Document{
		Name:         "intent1",
		Namespace:    "ns1",
		Priority:     10,
		NonRevertive: true,
		Orphan:       true,
		SensitivePaths: []*sdcpb.Path{
			{Elem: []*sdcpb.PathElem{{Name: "secret"}}},
		},
		Config: []*ConfigBlob{
			{Path: "/interface[name=eth0]/description", Value: []byte(`"uplink"`)},
		},
	}

	got, err := r.Get(context.Background(), target, "intent1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestGRPCConfigClient_Get_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	client.EXPECT().Get(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "no such Config"))

	r := NewGRPCConfigClient(nil)
	r.client = client

	_, err := r.Get(context.Background(), target, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestGRPCConfigClient_Get_OtherError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	wantErr := status.Error(codes.Unavailable, "controller unreachable")
	client.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, wantErr)

	r := NewGRPCConfigClient(nil)
	r.client = client

	_, err := r.Get(context.Background(), target, "intent1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Get() error = %v, want %v", err, wantErr)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error unexpectedly wrapped as ErrNotFound: %v", err)
	}
}

func TestGRPCConfigClient_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	client.EXPECT().List(gomock.Any(), &config_read.ListConfigRequest{
		TargetNamespace: "ns1",
		TargetName:      "target1",
	}).Return(&config_read.ListConfigResponse{
		Config: []*config_read.ConfigEntry{
			{Name: "intent1", Namespace: "ns1", Priority: 1},
			{Name: "intent2", Namespace: "ns1", Priority: 2},
		},
	}, nil)

	r := NewGRPCConfigClient(nil)
	r.client = client

	got, err := r.List(context.Background(), target)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var names []string
	for _, d := range got {
		names = append(names, d.IntentName())
	}
	if diff := cmp.Diff([]string{"ns1.intent1", "ns1.intent2"}, names); diff != "" {
		t.Errorf("List() names mismatch (-want +got):\n%s", diff)
	}
}

func TestGRPCConfigClient_List_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	client.EXPECT().List(gomock.Any(), gomock.Any()).
		Return(&config_read.ListConfigResponse{}, nil)

	r := NewGRPCConfigClient(nil)
	r.client = client

	got, err := r.List(context.Background(), target)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %v, want empty", got)
	}
}

func TestGRPCConfigClient_List_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mockconfigread.NewMockConfigSnapshotServiceClient(ctrl)
	target := Target{Namespace: "ns1", Name: "target1"}

	wantErr := status.Error(codes.Unavailable, "controller unreachable")
	client.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, wantErr)

	r := NewGRPCConfigClient(nil)
	r.client = client

	_, err := r.List(context.Background(), target)
	if !errors.Is(err, wantErr) {
		t.Fatalf("List() error = %v, want %v", err, wantErr)
	}
}
