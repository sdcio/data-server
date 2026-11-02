package datastore

// NorthboundSensitivityOpts controls whether real sensitive values are emitted on
// northbound export. The zero value keeps redaction (IncludeSensitive false).
type NorthboundSensitivityOpts struct {
	IncludeSensitive bool
}

// GetIntentOpts configures northbound export for one intent (or running).
type GetIntentOpts struct {
	IntentName string
	NorthboundSensitivityOpts
}

// BlameConfigOpts configures northbound blame export for the datastore.
type BlameConfigOpts struct {
	IncludeDefaults bool
	NorthboundSensitivityOpts
}
