package types

import "errors"

// ErrNotConnected indicates the southbound interface (device connection) of a
// datastore is not established
var ErrNotConnected = errors.New("not connected")

type TargetStatus struct {
	Status  TargetConnectionStatus
	Details string
}

func NewTargetStatus(status TargetConnectionStatus) *TargetStatus {
	return &TargetStatus{
		Status: status,
	}
}
func (ts *TargetStatus) IsConnected() bool {
	return ts.Status == TargetStatusConnected
}

type TargetConnectionStatus string

const (
	TargetStatusConnected    TargetConnectionStatus = "connected"
	TargetStatusNotConnected TargetConnectionStatus = "not connected"
)
