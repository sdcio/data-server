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

package utils

import (
	"reflect"
	"testing"

	"github.com/openconfig/gnmi/proto/gnmi"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestFromGNMIPath(t *testing.T) {
	type args struct {
		pre *gnmi.Path
		p   *gnmi.Path
	}
	tests := []struct {
		name string
		args args
		want *sdcpb.Path
	}{
		{
			name: "simple",
			args: args{
				// pre: &gnmi.Path{},
				p: &gnmi.Path{
					Elem: []*gnmi.PathElem{
						{Name: "foo"},
					},
				},
			},
			want: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					{Name: "foo"},
				},
				IsRootBased: true,
			},
		},
		{
			name: "two pe",
			args: args{
				// pre: &gnmi.Path{},
				p: &gnmi.Path{
					Elem: []*gnmi.PathElem{
						{Name: "foo"},
						{Name: "bar"},
					},
				},
			},
			want: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					{Name: "foo"},
					{Name: "bar"},
				},
				IsRootBased: true,
			},
		},
		{
			name: "two pe - key",
			args: args{
				// pre: &gnmi.Path{},
				p: &gnmi.Path{
					Elem: []*gnmi.PathElem{
						{Name: "foo", Key: map[string]string{"k": "v"}},
						{Name: "bar"},
					},
				},
			},
			want: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					{Name: "foo", Key: map[string]string{"k": "v"}},
					{Name: "bar"},
				},
				IsRootBased: true,
			},
		},
		{
			name: "two pe - 2 keys",
			args: args{
				// pre: &gnmi.Path{},
				p: &gnmi.Path{
					Elem: []*gnmi.PathElem{
						{Name: "foo", Key: map[string]string{"k1": "v1", "k2": "v2"}},
						{Name: "bar"},
					},
				},
			},
			want: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					{Name: "foo", Key: map[string]string{"k1": "v1", "k2": "v2"}},
					{Name: "bar"},
				},
				IsRootBased: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromGNMIPath(tt.args.pre, tt.args.p); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromGNMIPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
