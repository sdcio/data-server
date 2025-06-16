package types

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeviationEntry struct {
	intentName    string
	reason        DeviationReason
	path          *sdcpb.Path
	currentValue  *sdcpb.TypedValue
	expectedValue *sdcpb.TypedValue
}

func NewDeviationEntry(intentName string, reason DeviationReason, path *sdcpb.Path) *DeviationEntry {
	return &DeviationEntry{
		intentName: intentName,
		reason:     reason,
		path:       path,
	}
}

func (d *DeviationEntry) IntentName() string {
	return d.intentName
}
func (d *DeviationEntry) Reason() DeviationReason {
	return d.reason
}
func (d *DeviationEntry) Path() *sdcpb.Path {
	return d.path
}
func (d *DeviationEntry) CurrentValue() *sdcpb.TypedValue {
	return d.currentValue
}
func (d *DeviationEntry) ExpectedValue() *sdcpb.TypedValue {
	return d.expectedValue
}
func (d *DeviationEntry) SetCurrentValue(cv *sdcpb.TypedValue) *DeviationEntry {
	d.currentValue = cv
	return d
}
func (d *DeviationEntry) SetExpectedValue(ev *sdcpb.TypedValue) *DeviationEntry {
	d.expectedValue = ev
	return d
}
func (d *DeviationEntry) String() string {
	return fmt.Sprintf("Intent: %s, Path: %s, Reason: %d, Expected: %s, Current: %s", d.IntentName(), utils.ToStrings(d.Path(), false, false), d.Reason(), d.ExpectedValue(), d.CurrentValue())
}

type DeviationReason int

const (
	DeviationReasonUndefined DeviationReason = iota
	DeviationReasonUnhandled
	DeviationReasonNotApplied
	DeviationReasonOverruled
	DeviationReasonIntentExists
)
