package types

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/api"
)

// ErrNotConnected indicates the southbound interface (device connection) of a
// datastore is not established
var ErrNotConnected = api.ErrNotConnected

// TargetStatus is the southbound connection state of a target. The zero value
// is api.ConnectionUnknown
type TargetStatus struct {
	Status  api.ConnectionState
	Details string
}

func NewTargetStatus(status api.ConnectionState) *TargetStatus {
	return &TargetStatus{
		Status: status,
	}
}
func (ts *TargetStatus) IsConnected() bool {
	return ts.Status == api.ConnectionConnected
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
