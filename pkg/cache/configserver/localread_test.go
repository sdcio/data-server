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

import "testing"

func TestDocument_IntentName(t *testing.T) {
	cases := map[string]struct {
		doc  Document
		want string
	}{
		"gvk nsn": {
			doc:  Document{Namespace: "default", Name: "srl2-config"},
			want: "default.srl2-config",
		},
		"bare name when namespace missing": {
			doc:  Document{Name: "srl2-config"},
			want: "srl2-config",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.doc.IntentName(); got != tc.want {
				t.Errorf("IntentName() = %q, want %q", got, tc.want)
			}
		})
	}
}
