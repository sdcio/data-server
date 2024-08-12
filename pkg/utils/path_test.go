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
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestStripPathElemPrefix(t *testing.T) {
	type args struct {
		p string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "non-namespaced abs-path",
			args: args{
				p: "/foo/bar/bla",
			},
			want:    "/foo/bar/bla",
			wantErr: false,
		},
		{
			name: "namespaced abs-path",
			args: args{
				p: "/a:foo/somens:bar/somens:bla",
			},
			want:    "/foo/bar/bla",
			wantErr: false,
		},
		{
			name: "namespaced abs-path with key",
			args: args{
				p: "/a:foo/somens:bar[k=v]/somens:bla",
			},
			want:    "/foo/bar[k=v]/bla",
			wantErr: false,
		},
		{
			name: "namespaced abs-path with namespaced key",
			args: args{
				p: "/a:foo/somens:bar[somens:k=v]/somens:bla",
			},
			want:    "/foo/bar[k=v]/bla",
			wantErr: false,
		},
		{
			name: "namespaced rel-path",
			args: args{
				p: "a:foo/somens:bar/somens:bla",
			},
			want:    "foo/bar/bla",
			wantErr: false,
		},
		{
			name: "namespaced rel-path",
			args: args{
				p: "a:foo/somens:bar/somens:bla",
			},
			want:    "foo/bar/bla",
			wantErr: false,
		},
		{
			name: "namespaced rel-path with key",
			args: args{
				p: "a:foo/somens:bar[k=v]/somens:bla",
			},
			want:    "foo/bar[k=v]/bla",
			wantErr: false,
		},
		{
			name: "namespaced rel-path with namespaced key",
			args: args{
				p: "a:foo/somens:bar[somens:k=v]/somens:bla",
			},
			want:    "foo/bar[k=v]/bla",
			wantErr: false,
		},
		{
			name: "malformed path with {",
			args: args{
				p: "/a:foo/somens:bar{somens:k=v]/somens:bla",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "path with current() function in key",
			args: args{
				p: "/srl_nokia-netinst:network-instance[srl_nokia-netinst:name = current()/../../../../../srl_nokia-netinst:name]/srl_nokia-netinst:interface/srl_nokia-netinst:name",
			},
			want:    "/network-instance[name=current()/../../../../../name]/interface/name",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StripPathElemPrefix(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("StripPathElemPrefix() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("StripPathElemPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToXPath(t *testing.T) {
	type args struct {
		p      *sdcpb.Path
		noKeys bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Path without keys, noKeys = false",
			args: args{
				noKeys: false,
				p: &sdcpb.Path{
					Elem: []*sdcpb.PathElem{
						{
							Name: "One",
						},
						{
							Name: "Two",
						},
						{
							Name: "Three",
						},
					},
				},
			},
			want: "One/Two/Three",
		},
		{
			name: "Path without keys, noKeys = true",
			args: args{
				noKeys: true,
				p: &sdcpb.Path{
					Elem: []*sdcpb.PathElem{
						{
							Name: "One",
						},
						{
							Name: "Two",
						},
						{
							Name: "Three",
						},
					},
				},
			},
			want: "One/Two/Three",
		},
		{
			name: "Path with keys, NoKeys = True",
			args: args{
				noKeys: true,
				p: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					{Name: "interface", Key: map[string]string{
						"name": "ethernet-1/1",
					}},
					{
						Name: "subinterface", Key: map[string]string{
							"index": "0",
						},
					},
					{
						Name: "admin-state",
					},
				}},
			},
			want: "interface/subinterface/admin-state",
		},
		{
			name: "Path with keys, NoKeys = false",
			args: args{
				noKeys: false,
				p: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					{Name: "interface", Key: map[string]string{
						"name": "ethernet-1/1",
					}},
					{
						Name: "subinterface", Key: map[string]string{
							"index": "0",
						},
					},
					{
						Name: "admin-state",
					},
				}},
			},
			want: "interface[name=ethernet-1/1]/subinterface[index=0]/admin-state",
		},
		{
			name: "Path with multiple keys, NoKeys = True",
			args: args{
				noKeys: true,
				p: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					{Name: "interface", Key: map[string]string{
						"name":      "ethernet-1/1",
						"schnitzel": "foo",
					}},
					{
						Name: "subinterface", Key: map[string]string{
							"index": "0",
						},
					},
					{
						Name: "admin-state",
					},
				}},
			},
			want: "interface/subinterface/admin-state",
		},
		{
			name: "Path with multiple  keys, NoKeys = false",
			args: args{
				noKeys: false,
				p: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					{Name: "interface", Key: map[string]string{
						"name":      "ethernet-1/1",
						"schnitzel": "foo",
					}},
					{
						Name: "subinterface", Key: map[string]string{
							"index": "0",
						},
					},
					{
						Name: "admin-state",
					},
				}},
			},
			want: "interface[name=ethernet-1/1][schnitzel=foo]/subinterface[index=0]/admin-state",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToXPath(tt.args.p, tt.args.noKeys); got != tt.want {
				t.Errorf("ToXPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
