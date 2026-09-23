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

package cache

import (
	"errors"
	"testing"

	"github.com/sdcio/data-server/pkg/cache/configserver"
)

func TestSplitDatastoreName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    configserver.Target
		wantErr bool
	}{
		{
			name: "dotted",
			in:   "prod.srl1",
			want: configserver.Target{Namespace: "prod", Name: "srl1"},
		},
		{
			name: "dotted with extra dots in name",
			in:   "prod.rack1.srl1",
			want: configserver.Target{Namespace: "prod", Name: "rack1.srl1"},
		},
		{
			name:    "no dot",
			in:      "srl1",
			wantErr: true,
		},
		{
			name:    "empty namespace",
			in:      ".srl1",
			wantErr: true,
		},
		{
			name:    "empty name",
			in:      "prod.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitDatastoreName(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrMalformedDatastoreName) {
					t.Fatalf("splitDatastoreName(%q) error = %v, want ErrMalformedDatastoreName", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitDatastoreName(%q) unexpected error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("splitDatastoreName(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLookupConfigName(t *testing.T) {
	target := configserver.Target{Namespace: "prod", Name: "srl1"}

	tests := []struct {
		name       string
		intentName string
		want       string
	}{
		{
			name:       "prefix match",
			intentName: "prod.intent1",
			want:       "intent1",
		},
		{
			name:       "prefix match with dots in rest",
			intentName: "prod.rack1.intent1",
			want:       "rack1.intent1",
		},
		{
			name:       "bare pass-through",
			intentName: "intent1",
			want:       "intent1",
		},
		{
			name:       "other namespace pass-through",
			intentName: "other.intent1",
			want:       "other.intent1",
		},
		{
			name:       "empty rest",
			intentName: "prod.",
			want:       "prod.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lookupConfigName(target, tt.intentName)
			if got != tt.want {
				t.Errorf("lookupConfigName(%+v, %q) = %q, want %q", target, tt.intentName, got, tt.want)
			}
		})
	}
}
