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
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func Test_isTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "raw io.EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "wrapped io.EOF",
			err:  fmt.Errorf("reading response: %w", io.EOF),
			want: true,
		},
		{
			name: "net.Error",
			err:  &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
			want: true,
		},
		{
			name: "broken pipe substring",
			err:  errors.New("write: broken pipe"),
			want: true,
		},
		{
			name: "connection reset substring",
			err:  errors.New("read: connection reset by peer"),
			want: true,
		},
		{
			name: "i/o timeout substring",
			err:  errors.New("read: i/o timeout"),
			want: true,
		},
		{
			name: "literal EOF substring",
			err:  errors.New("unexpected EOF while parsing"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("invalid value"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransportError(tt.err); got != tt.want {
				t.Errorf("isTransportError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
