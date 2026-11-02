// Package sensitive owns northbound redaction of sensitive leaf values: the
// redaction sentinel, the live index of sensitive path markers, and the Render
// that resolves a leaf to redacted-or-real. Every northbound render, blame and
// deviation operation redacts through Render and nothing else.
package sensitive

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RedactedStringValue is the sentinel substituted in place of sensitive leaf
// values in deviation, render, and blame outputs.
const RedactedStringValue = "***"

// RedactedTypedValue is the canonical TypedValue emitted in place of a
// sensitive leaf. Callers must not mutate the returned pointer.
var RedactedTypedValue = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: RedactedStringValue}}

// Leaf is the part of a tree entry that redaction needs. It is satisfied by
// api.Entry, which keeps this package free of tree dependencies.
type Leaf interface {
	GetSchema() *sdcpb.SchemaElem
	SdcpbPath() *sdcpb.Path
}

// Render is the single northbound redaction context: an include-sensitive
// bypass plus a path-marker checker. It resolves a leaf to redacted-or-real
// so formatters only format.
//
// Build it with NewRender (northbound, honours markers) or RevealAll
// (southbound, secrets pass through in plaintext). The zero value redacts
// schema-sensitive leaves only.
type Render struct {
	includeSensitive bool
	paths            PathChecker
}

// NewRender builds a redaction context. includeSensitive=true reveals all
// values; paths is the path-marker checker (nil skips marker-based redaction).
// For northbound APIs paths is the datastore's live PathIndex.
func NewRender(includeSensitive bool, paths PathChecker) Render {
	return Render{includeSensitive: includeSensitive, paths: paths}
}

// RevealAll builds a Render that never redacts, for southbound emission
// (sync / device output) where secrets must reach the device.
func RevealAll() Render {
	return Render{includeSensitive: true}
}

// ShouldRedact reports whether the value of leaf must be replaced with the
// redaction sentinel. A leaf is redacted unless the caller opted in to real
// values, and either its schema is flagged sensitive or its key-pruned path
// is marked in the path checker.
func (r Render) ShouldRedact(leaf Leaf) bool {
	if r.includeSensitive {
		return false
	}
	if s := leaf.GetSchema(); s != nil && s.IsSensitive() {
		return true
	}
	return r.paths != nil && r.paths.Contains(leaf.SdcpbPath())
}

// String returns the redaction sentinel when leaf must be redacted, otherwise real.
func (r Render) String(leaf Leaf, real string) string {
	if r.ShouldRedact(leaf) {
		return RedactedStringValue
	}
	return real
}

// TypedValue returns the redaction sentinel when leaf must be redacted, otherwise real.
func (r Render) TypedValue(leaf Leaf, real *sdcpb.TypedValue) *sdcpb.TypedValue {
	if r.ShouldRedact(leaf) {
		return RedactedTypedValue
	}
	return real
}
