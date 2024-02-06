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

import "testing"

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
