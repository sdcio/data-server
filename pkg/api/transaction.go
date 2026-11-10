package api

import (
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// Origin identifies the requester of an operation for logging and audit. It is
// optional; the gRPC adapter fills it from the peer and in-process callers set
// their own name.
type Origin string

// TxRef addresses a transaction within a datastore.
type TxRef struct {
	Datastore string
	ID        string
}

// SetOptions tune a transaction set.
type SetOptions struct {
	// DryRun validates without applying.
	DryRun bool
	// Timeout auto-cancels an unconfirmed transaction; zero means the default.
	Timeout time.Duration
	Origin  Origin
	// Replace, when set, replaces the whole device configuration with its
	// content. Its name and priority are ignored.
	Replace *Intent
}

// IntentSpec is the body of an Intent: exactly one of Update and Delete is set.
type IntentSpec struct {
	Update *Update
	Delete *Delete
}

// Intent is a named unit of desired configuration within a transaction.
type Intent struct {
	Name string
	Spec IntentSpec
}

// Update creates or replaces an intent.
type Update struct {
	Priority int32
	// Updates are the intent body. A JSON value expands below its path into
	// typed leaf values; other value types are taken as given.
	Updates []*sdcpb.Update
	// Deletes are paths removed explicitly on behalf of the intent.
	Deletes []*sdcpb.Path
	// RevertPaths limits reverting to these paths for a non-revertive intent.
	RevertPaths []*sdcpb.Path
	// NonRevertive leaves device-side changes in place instead of reverting them.
	NonRevertive bool
	// PreviouslyApplied marks a non-revertive intent that was applied before.
	PreviouslyApplied bool
}

// Delete removes an intent.
type Delete struct {
	// IgnoreNoExist suppresses the error for an intent that does not exist.
	IgnoreNoExist bool
	// Orphan removes the intent from the intended store but keeps device config.
	Orphan bool
}

// Format selects the encoding of intent and blame output.
type Format int

const (
	FormatJSON Format = iota
	FormatJSONIETF
	FormatXML
	FormatProto
	FormatXPath
)

// IntentRequest identifies an intent to read back.
type IntentRequest struct {
	Datastore string
	Intent    string
	Format    Format
	Origin    Origin
}

// IntentResult is an intent read back from the intended store. Content holds
// the intent encoded as requested: JSON, JSON IETF, XML or protojson encoded
// xpath/value pairs; for FormatProto it is a proto encoded sdcpb.Intent.
type IntentResult struct {
	Name         string
	Priority     int32
	Orphan       bool
	NonRevertive bool
	Deletes      []*sdcpb.Path
	Format       Format
	Content      []byte
}

// IntentValidation holds the findings of one intent in a transaction.
type IntentValidation struct {
	Warnings []string
	Errors   []string
}

// PathValue is a path with its typed value.
type PathValue struct {
	Path  *sdcpb.Path
	Value *sdcpb.TypedValue
}

// TransactionResult is the outcome of a transaction set. Validation failures
// are reported in Intents, not as an error.
type TransactionResult struct {
	Updates  []PathValue
	Deletes  []*sdcpb.Path
	Intents  map[string]IntentValidation
	Warnings []string
}

// BlameNode is one level of the blame tree: which intent owns the value below
// a path. Values keep their original type.
type BlameNode struct {
	Name           string
	Owner          string
	Value          *sdcpb.TypedValue
	DeviationValue *sdcpb.TypedValue
	// KeyName is set when the level is the key level of a list.
	KeyName  string
	Children []*BlameNode
}
