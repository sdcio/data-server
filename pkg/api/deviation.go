package api

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
	DeviationReasonIntentExists
)

// DeviationEntry is one deviating path with its expected and current values.
// Expected and Current are display strings; ExpectedText and CurrentText carry
// the same values in the stable text encoding that stored deviations use. An
// absent value is empty in both forms.
type DeviationEntry struct {
	Path         string
	Expected     string
	Current      string
	ExpectedText string
	CurrentText  string
}

// DeviationEvent is emitted by a deviation watch. Intent, Reason and Entries
// are set only where the Kind carries them.
type DeviationEvent struct {
	Datastore string
	Kind      DeviationKind
	Intent    string
	Reason    DeviationReason
	Entries   []DeviationEntry
}
