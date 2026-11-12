package types

// SetResult is the outcome of a southbound Set. Warnings are non-fatal
// messages reported by the device.
type SetResult struct {
	Warnings []string
}
