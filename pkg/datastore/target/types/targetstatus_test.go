package types

import (
	"errors"
	"strings"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestTargetStatusErr(t *testing.T) {
	cases := map[string]struct {
		status      *TargetStatus
		wantNil     bool
		wantDetails string
	}{
		"connected": {
			status:  NewTargetStatus(sdcpb.TargetStatus_CONNECTED),
			wantNil: true,
		},
		"connected with details": {
			status:  &TargetStatus{Status: sdcpb.TargetStatus_CONNECTED, Details: "READY"},
			wantNil: true,
		},
		"not connected": {
			status: NewTargetStatus(sdcpb.TargetStatus_NOT_CONNECTED),
		},
		"not connected with details": {
			status:      &TargetStatus{Status: sdcpb.TargetStatus_NOT_CONNECTED, Details: "connection not initialized"},
			wantDetails: "connection not initialized",
		},
		"unknown": {
			status: NewTargetStatus(sdcpb.TargetStatus_UNKNOWN),
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

	if ts.Status != sdcpb.TargetStatus_UNKNOWN {
		t.Fatalf("expected zero value to be UNKNOWN, got %v", ts.Status)
	}
	if ts.IsConnected() {
		t.Fatal("expected zero value to report not connected")
	}
}
