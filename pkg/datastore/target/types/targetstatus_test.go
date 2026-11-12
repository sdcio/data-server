package types

import (
	"errors"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/api"
)

func TestTargetStatusErr(t *testing.T) {
	cases := map[string]struct {
		status      *TargetStatus
		wantNil     bool
		wantDetails string
	}{
		"connected": {
			status:  NewTargetStatus(api.ConnectionConnected),
			wantNil: true,
		},
		"connected with details": {
			status:  &TargetStatus{Status: api.ConnectionConnected, Details: "READY"},
			wantNil: true,
		},
		"not connected": {
			status: NewTargetStatus(api.ConnectionNotConnected),
		},
		"not connected with details": {
			status:      &TargetStatus{Status: api.ConnectionNotConnected, Details: "connection not initialized"},
			wantDetails: "connection not initialized",
		},
		"unknown": {
			status: NewTargetStatus(api.ConnectionUnknown),
		},
		"zero value defaults to unknown": {
			status: &TargetStatus{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.status.Err()

			if tc.wantNil {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}

			if !errors.Is(err, ErrNotConnected) {
				t.Fatalf("expected error matching ErrNotConnected, got %v", err)
			}
			if tc.wantDetails != "" && !strings.Contains(err.Error(), tc.wantDetails) {
				t.Fatalf("expected error to contain %q, got %q", tc.wantDetails, err.Error())
			}
		})
	}
}

// The zero value must not read as connected, otherwise a status that was never
// populated would let a write through to a device.
func TestTargetStatusZeroValueIsNotConnected(t *testing.T) {
	var ts TargetStatus

	if ts.Status != api.ConnectionUnknown {
		t.Fatalf("expected zero value to be UNKNOWN, got %v", ts.Status)
	}
	if ts.IsConnected() {
		t.Fatal("expected zero value to report not connected")
	}
}
