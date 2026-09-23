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

package scrapligo

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/scrapli/scrapligo/util"
)

func Test_normalizeTransportError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantNil    bool
		wantEOF    bool
		wantSameAs error
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "scrapligo connection error is normalized to io.EOF",
			err:     util.ErrConnectionError,
			wantEOF: true,
		},
		{
			name:    "wrapped scrapligo connection error is still normalized to io.EOF",
			err:     fmt.Errorf("channel read loop failed: %w", util.ErrConnectionError),
			wantEOF: true,
		},
		{
			name:       "unrelated scrapligo error is returned unchanged",
			err:        util.ErrTimeoutError,
			wantSameAs: util.ErrTimeoutError,
		},
		{
			name:       "generic error is returned unchanged",
			err:        errors.New("some other failure"),
			wantSameAs: nil, // checked by message equality below instead
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTransportError(tt.err)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}

			if tt.wantEOF {
				if !errors.Is(got, io.EOF) {
					t.Errorf("expected errors.Is(got, io.EOF), got %v", got)
				}
				// the original scrapligo error must still be discoverable so
				// logs/debugging don't lose the underlying detail.
				if !errors.Is(got, util.ErrConnectionError) {
					t.Errorf("expected the original util.ErrConnectionError to still be discoverable via errors.Is, got %v", got)
				}
				return
			}

			if tt.wantSameAs != nil {
				if !errors.Is(got, tt.wantSameAs) {
					t.Errorf("expected error to be returned unchanged, got %v want %v", got, tt.wantSameAs)
				}
				if errors.Is(got, io.EOF) {
					t.Errorf("did not expect a non-connection error to be normalized to io.EOF, got %v", got)
				}
				return
			}

			// generic error case: returned unchanged (same error value).
			if got != tt.err {
				t.Errorf("expected the original error to be returned unchanged, got %v want %v", got, tt.err)
			}
		})
	}
}
