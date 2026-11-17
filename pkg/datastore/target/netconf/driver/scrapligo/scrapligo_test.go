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
	"os"
	"testing"
	"time"

	"github.com/scrapli/scrapligo/util"

	"github.com/sdcio/data-server/pkg/config"
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

func newTestTarget(t *testing.T, s *fakeNetconfServer, timeout time.Duration) *ScrapligoNetconfTarget {
	t.Helper()
	d, err := NewScrapligoNetconfTarget(&config.SBI{
		Address:        "127.0.0.1",
		Port:           uint32(s.addr().Port),
		Timeout:        timeout,
		NetconfOptions: &config.SBINetconfOptions{},
	})
	if err != nil {
		t.Fatalf("connecting to fake server: %v", err)
	}
	return d
}

func closeWithin(t *testing.T, d *ScrapligoNetconfTarget, bound time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { _ = d.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(bound):
		t.Fatalf("Close did not return within %s", bound)
	}
}

func Test_Close_HealthyConnection(t *testing.T) {
	s := newFakeNetconfServer(t)
	d := newTestTarget(t, s, 5*time.Second)
	if _, err := d.GetConfig("running", ""); err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	closeWithin(t, d, time.Second)
}

func Test_Close_ReturnsAfterPeerDies(t *testing.T) {
	s := newFakeNetconfServer(t)
	d := newTestTarget(t, s, 5*time.Second)
	if _, err := d.GetConfig("running", ""); err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	s.kill()
	time.Sleep(200 * time.Millisecond)

	_, err := d.GetConfig("running", "")
	if err == nil {
		t.Fatal("expected an error after the peer died")
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected a transport error classifiable as io.EOF, got %v", err)
	}
	closeWithin(t, d, closeTimeout+2*time.Second)
}

func Test_BlackholedPeer_SurfacesAsDeadlineExceeded(t *testing.T) {
	s := newFakeNetconfServer(t)
	d := newTestTarget(t, s, 500*time.Millisecond)
	s.setBlackhole(true)

	_, err := d.GetConfig("running", "")
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("expected os.ErrDeadlineExceeded, got %v", err)
	}
	closeWithin(t, d, closeTimeout+2*time.Second)
}
