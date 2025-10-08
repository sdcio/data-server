package types

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
