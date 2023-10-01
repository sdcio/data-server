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
