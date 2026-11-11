package api

import sdcpb "github.com/sdcio/sdc-protos/sdcpb"

// DeviationKind classifies a DeviationEvent: Start opens a deviation stream
// for a datastore, Update reports a changed deviation, End closes the stream
// normally and Clear drops all deviations previously reported for the intent.
type DeviationKind int

const (
	DeviationStart DeviationKind = iota
	DeviationUpdate
	DeviationEnd
	DeviationClear
)

// DeviationReason explains why a deviation exists.
type DeviationReason int

const (
	DeviationReasonUnknown DeviationReason = iota
	DeviationReasonUnhandled
	DeviationReasonNotApplied
	DeviationReasonOverruled
	// DeviationReasonIntentExists is not a deviation reason: it marks that the
	// named intent exists, and is sent without path or values.
	DeviationReasonIntentExists
)

// DeviationEvent is emitted by a deviation watch, one per deviating path.
// Intent, Reason, Path, Expected and Current are set only where the Kind
// carries them.
type DeviationEvent struct {
	Datastore string
	Kind      DeviationKind
	Intent    string
	Reason    DeviationReason
	Path      *sdcpb.Path
	Expected  *sdcpb.TypedValue
	Current   *sdcpb.TypedValue
}
