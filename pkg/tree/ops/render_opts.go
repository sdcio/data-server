package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// SensitiveRender is the single tree-owned northbound redaction context:
// expose/include-sensitive flag plus a path-marker checker. It resolves a leaf
// to redacted-or-real so formatters only format.
type SensitiveRender struct {
	includeSensitive bool
	paths            types.SensitivePathChecker
}

// NewSensitiveRender builds a redaction context. includeSensitive=true reveals
// all values; paths is the path-marker checker (nil skips marker-based redaction).
func NewSensitiveRender(includeSensitive bool, paths types.SensitivePathChecker) SensitiveRender {
	return SensitiveRender{includeSensitive: includeSensitive, paths: paths}
}

// ShouldRedact reports whether the leaf at e must be replaced with the
// redaction sentinel. Delegates to types.ShouldRedact.
func (s SensitiveRender) ShouldRedact(e api.Entry) bool {
	return types.ShouldRedact(s.includeSensitive, e.GetSchema(), e.SdcpbPath(), s.paths)
}

// String returns the redaction sentinel when the leaf must be redacted, otherwise real.
func (s SensitiveRender) String(e api.Entry, real string) string {
	if s.ShouldRedact(e) {
		return types.RedactedStringValue
	}
	return real
}

// TypedValue returns the redaction sentinel when the leaf must be redacted, otherwise real.
func (s SensitiveRender) TypedValue(e api.Entry, real *sdcpb.TypedValue) *sdcpb.TypedValue {
	if s.ShouldRedact(e) {
		return types.RedactedTypedValue
	}
	return real
}

// RenderOpts holds common options shared across all northbound render operations.
// Sensitive redaction is owned by the embedded SensitiveRender; use
// RenderOptsNorthbound or RenderOptsRevealAll instead of setting fields by hand.
type RenderOpts struct {
	OnlyNewOrUpdated bool
	SensitiveRender
}

// RenderOptsNorthbound builds opts for northbound APIs (GetIntent, BlameConfig,
// WatchDeviations). live is the Live Sensitive Path Index.
func RenderOptsNorthbound(includeSensitive bool, live types.SensitivePathChecker) RenderOpts {
	return RenderOpts{SensitiveRender: NewSensitiveRender(includeSensitive, live)}
}

// RenderOptsRevealAll builds opts for southbound emission (sync / device output)
// where secrets must pass through in plaintext.
func RenderOptsRevealAll() RenderOpts {
	return RenderOpts{SensitiveRender: NewSensitiveRender(true, nil)}
}

// WithOnlyNewOrUpdated returns a copy of opts with OnlyNewOrUpdated set.
func (o RenderOpts) WithOnlyNewOrUpdated(onlyNewOrUpdated bool) RenderOpts {
	o.OnlyNewOrUpdated = onlyNewOrUpdated
	return o
}

// XMLRenderOpts extends RenderOpts with XML-specific flags.
type XMLRenderOpts struct {
	RenderOpts
	HonorNamespace         bool
	OperationWithNamespace bool
	UseOperationRemove     bool
}

// XPathRenderOpts extends RenderOpts with XPath-specific flags.
type XPathRenderOpts struct {
	RenderOpts
	IncludeDefaults bool
}
