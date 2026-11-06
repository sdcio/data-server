package api

// DeviationKind classifies a DeviationEvent.
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
type DeviationEntry struct {
	Path     string
	Expected string
	Current  string
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
