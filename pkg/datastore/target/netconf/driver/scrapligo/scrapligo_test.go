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

	"github.com/scrapli/scrapligo/response"
	"github.com/scrapli/scrapligo/util"

	"github.com/sdcio/data-server/pkg/config"
	nctypes "github.com/sdcio/data-server/pkg/datastore/target/netconf/types"
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

func Test_parseReply(t *testing.T) {
	const capsChanged = `<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1">` +
		`<rpc-error>` +
		`<error-type>application</error-type>` +
		`<error-tag>resource-denied</error-tag>` +
		`<error-severity>error</error-severity>` +
		`<error-app-tag>capabilities-changed</error-app-tag>` +
		`</rpc-error>` +
		`</rpc-reply>`
	const prefixed = `<nc:rpc-reply xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1">` +
		`<nc:rpc-error>` +
		`<nc:error-type>application</nc:error-type>` +
		`<nc:error-tag> resource-denied </nc:error-tag>` +
		`<nc:error-severity>error</nc:error-severity>` +
		`<nc:error-app-tag> capabilities-changed </nc:error-app-tag>` +
		`</nc:rpc-error>` +
		`</nc:rpc-reply>`
	const warningThenCapsChanged = `<rpc-reply message-id="1">` +
		`<rpc-error><error-tag>operation-failed</error-tag><error-severity>warning</error-severity></rpc-error>` +
		`<rpc-error><error-type>application</error-type><error-tag>resource-denied</error-tag><error-severity>error</error-severity><error-app-tag>capabilities-changed</error-app-tag></rpc-error>` +
		`</rpc-reply>`
	const warningSeverity = `<rpc-reply message-id="1">` +
		`<rpc-error><error-type>application</error-type><error-tag>resource-denied</error-tag><error-severity>warning</error-severity><error-app-tag>capabilities-changed</error-app-tag></rpc-error>` +
		`</rpc-reply>`
	const tagOnly = `<rpc-reply message-id="1"><rpc-error><error-tag>resource-denied</error-tag><error-severity>error</error-severity></rpc-error></rpc-reply>`
	const appTagOnly = `<rpc-reply message-id="1"><rpc-error><error-tag>operation-failed</error-tag><error-severity>error</error-severity><error-app-tag>capabilities-changed</error-app-tag></rpc-error></rpc-reply>`
	const splitAcrossErrors = `<rpc-reply message-id="1">` +
		`<rpc-error><error-tag>resource-denied</error-tag><error-severity>error</error-severity></rpc-error>` +
		`<rpc-error><error-tag>operation-failed</error-tag><error-severity>error</error-severity><error-app-tag>capabilities-changed</error-app-tag></rpc-error>` +
		`</rpc-reply>`
	const ok = `<rpc-reply message-id="1"><ok/></rpc-reply>`
	const prefixedError = `<nc:rpc-reply xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1">` +
		`<nc:rpc-error><nc:error-tag>invalid-value</nc:error-tag><nc:error-severity>error</nc:error-severity></nc:rpc-error>` +
		`</nc:rpc-reply>`
	const nested = `<rpc-reply message-id="1"><commit-results>` +
		`<rpc-error><error-severity>error</error-severity><error-message>commit failed</error-message></rpc-error>` +
		`</commit-results></rpc-reply>`
	const noSeverity = `<rpc-reply message-id="1"><rpc-error><error-tag>invalid-value</error-tag></rpc-error></rpc-reply>`
	const errorInData = `<rpc-reply message-id="1"><data><audit xmlns="urn:example">` +
		`<rpc-error><error-severity>error</error-severity><error-message>stored</error-message></rpc-error>` +
		`</audit></data></rpc-reply>`
	const errorInPrefixedData = `<nc:rpc-reply xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><nc:data>` +
		`<nc:rpc-error><nc:error-severity>error</nc:error-severity></nc:rpc-error>` +
		`</nc:data></nc:rpc-reply>`
	const errorBesideData = `<rpc-reply message-id="1"><data><interface/></data>` +
		`<rpc-error><error-severity>error</error-severity></rpc-error></rpc-reply>`

	tests := []struct {
		name     string
		result   string
		failed   bool // whether scrapligo set resp.Failed
		wantErr  bool
		wantCaps bool
	}{
		{"ok", ok, false, false, false},
		{"warning severity only", warningSeverity, true, false, false},
		{"capabilities-changed", capsChanged, true, true, true},
		{"namespace-prefixed with whitespace", prefixed, true, true, true},
		{"warning followed by capabilities-changed", warningThenCapsChanged, true, true, true},
		{"resource-denied alone", tagOnly, true, true, false},
		{"app-tag alone", appTagOnly, true, true, false},
		{"tags spread over different rpc-errors", splitAcrossErrors, true, true, false},
		{"namespace-prefixed error", prefixedError, true, true, false},
		{"rpc-error nested in a result element", nested, true, true, false},
		{"rpc-error without severity", noSeverity, true, true, false},
		{"rpc-error scrapligo did not flag", tagOnly, false, true, false},
		{"rpc-error element inside data", errorInData, true, false, false},
		{"rpc-error element inside prefixed data", errorInPrefixedData, true, false, false},
		{"rpc-error next to data", errorBesideData, true, true, false},
		{"malformed xml", "<rpc-reply><rpc-error>", true, true, false},
		{"empty result after framing error", "", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &response.NetconfResponse{Result: tt.result}
			failed := errors.New("rpc failed")
			if tt.failed {
				resp.Failed = failed
			}
			doc, err := parseReply(resp)
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if doc == nil {
					t.Error("expected the parsed reply on success")
				}
				return
			}
			if tt.failed && !errors.Is(err, failed) {
				t.Errorf("original failure must stay discoverable, got %v", err)
			}
			if gotCaps := errors.Is(err, nctypes.ErrCapabilitiesChanged); gotCaps != tt.wantCaps {
				t.Errorf("ErrCapabilitiesChanged = %v, want %v", gotCaps, tt.wantCaps)
			}
		})
	}
}

func Test_RPCErrors_SameForAllMethods_OverSSH(t *testing.T) {
	s := newFakeNetconfServer(t)
	d := newTestTarget(t, s, 5*time.Second)
	defer closeWithin(t, d, time.Second)

	calls := map[string]func() error{
		"GetConfig":  func() error { _, err := d.GetConfig("running", ""); return err },
		"EditConfig": func() error { _, err := d.EditConfig("candidate", "<interface/>"); return err },
		"Commit":     func() error { _, err := d.Commit(); return err },
		"Discard":    d.Discard,
		"Validate":   func() error { _, err := d.Validate("candidate"); return err },
	}

	s.setReplyBody(`<data/><rpc-error><error-severity>warning</error-severity><error-message>deprecated</error-message></rpc-error>`)
	for name, call := range calls {
		if err := call(); err != nil {
			t.Errorf("%s: warning-only reply must succeed, got %v", name, err)
		}
	}

	// the inline xmlns keeps it out of scrapligo's literal <nc:rpc-error> match
	s.setReplyBody(`<nc:rpc-error xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0">` +
		`<nc:error-tag>invalid-value</nc:error-tag><nc:error-severity>error</nc:error-severity>` +
		`</nc:rpc-error>`)
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: prefixed error-severity rpc-error must fail", name)
		}
	}

	// scrapligo flags this reply as failed, but <data> is device content
	s.setReplyBody(`<data><rpc-error><error-severity>error</error-severity></rpc-error></data>`)
	for name, call := range calls {
		if err := call(); err != nil {
			t.Errorf("%s: rpc-error element inside <data> must not fail, got %v", name, err)
		}
	}
}

func Test_CapabilitiesChanged_OverSSH(t *testing.T) {
	s := newFakeNetconfServer(t)
	d := newTestTarget(t, s, 5*time.Second)
	defer closeWithin(t, d, time.Second)

	s.setReplyBody(`<rpc-error>` +
		`<error-type>application</error-type>` +
		`<error-tag>resource-denied</error-tag>` +
		`<error-severity>error</error-severity>` +
		`<error-app-tag>capabilities-changed</error-app-tag>` +
		`</rpc-error>`)

	if _, err := d.GetConfig("running", ""); !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Errorf("GetConfig error = %v, want ErrCapabilitiesChanged", err)
	}
	if _, err := d.EditConfig("running", "<interface/>"); !errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Errorf("EditConfig error = %v, want ErrCapabilitiesChanged", err)
	}

	s.setReplyBody(`<rpc-error>` +
		`<error-type>application</error-type>` +
		`<error-tag>resource-denied</error-tag>` +
		`<error-severity>error</error-severity>` +
		`</rpc-error>`)

	_, err := d.GetConfig("running", "")
	if err == nil {
		t.Fatal("expected GetConfig error for resource-denied")
	}
	if errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.Errorf("plain resource-denied must not match ErrCapabilitiesChanged, got %v", err)
	}
}
