package types

import (
	"errors"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ErrNotConnected indicates the southbound interface (device connection) of a
// datastore is not established
var ErrNotConnected = errors.New("not connected")

// TargetStatus is the southbound connection state of a target. The zero value
// is sdcpb.TargetStatus_UNKNOWN
type TargetStatus struct {
	Status  sdcpb.TargetStatus
	Details string
}

func NewTargetStatus(status sdcpb.TargetStatus) *TargetStatus {
	return &TargetStatus{
		Status: status,
	}
}
func (ts *TargetStatus) IsConnected() bool {
	return ts.Status == sdcpb.TargetStatus_CONNECTED
}

// Err reports the connection state as an error, so callers can propagate it and
// match it with errors.Is. It returns nil when the target is connected. This is
// the single place where a connection state turns into ErrNotConnected, keeping
// the Details a target collected attached to the error.
func (ts *TargetStatus) Err() error {
	if ts.IsConnected() {
		return nil
	}
	if ts.Details != "" {
		return fmt.Errorf("%w: %s", ErrNotConnected, ts.Details)
	}
	return ErrNotConnected
}
