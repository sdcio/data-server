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
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestFakeLocalConfigReader_GetNotFound(t *testing.T) {
	f := NewFakeLocalConfigReader()

	_, err := f.Get(context.Background(), Target{Namespace: "ns1", Name: "target1"}, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFakeLocalConfigReader_GetNotFound_UnknownTarget(t *testing.T) {
	f := NewFakeLocalConfigReader()
	f.Seed(Target{Namespace: "ns1", Name: "target1"}, &Document{Name: "intent1"})

	_, err := f.Get(context.Background(), Target{Namespace: "ns1", Name: "other-target"}, "intent1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFakeLocalConfigReader_SeedAndGet(t *testing.T) {
	f := NewFakeLocalConfigReader()
	target := Target{Namespace: "ns1", Name: "target1"}

	want := &Document{
		Name:         "intent1",
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
	f.Seed(target, want)

	got, err := f.Get(context.Background(), target, "intent1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestFakeLocalConfigReader_List(t *testing.T) {
	f := NewFakeLocalConfigReader()
	target := Target{Namespace: "ns1", Name: "target1"}
	other := Target{Namespace: "ns1", Name: "other-target"}

	f.Seed(target, &Document{Name: "intent2"}, &Document{Name: "intent1"})
	f.Seed(other, &Document{Name: "unrelated"})

	got, err := f.List(context.Background(), target)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var names []string
	for _, d := range got {
		names = append(names, d.Name)
	}
	if diff := cmp.Diff([]string{"intent1", "intent2"}, names); diff != "" {
		t.Errorf("List() names mismatch (-want +got):\n%s", diff)
	}
}

func TestFakeLocalConfigReader_ListEmptyForUnknownTarget(t *testing.T) {
	f := NewFakeLocalConfigReader()

	got, err := f.List(context.Background(), Target{Namespace: "ns1", Name: "target1"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() = %v, want empty", got)
	}
}

func TestFakeLocalConfigReader_SeedReplacesByName(t *testing.T) {
	f := NewFakeLocalConfigReader()
	target := Target{Namespace: "ns1", Name: "target1"}

	f.Seed(target, &Document{Name: "intent1", Priority: 1})
	f.Seed(target, &Document{Name: "intent1", Priority: 2})

	got, err := f.Get(context.Background(), target, "intent1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Priority != 2 {
		t.Errorf("Get().Priority = %d, want 2", got.Priority)
	}
}

func TestFakeLocalConfigReader_Reset(t *testing.T) {
	f := NewFakeLocalConfigReader()
	target := Target{Namespace: "ns1", Name: "target1"}
	f.Seed(target, &Document{Name: "intent1"})

	f.Reset()

	_, err := f.Get(context.Background(), target, "intent1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound after Reset", err)
	}
}
