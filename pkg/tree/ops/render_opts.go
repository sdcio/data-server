package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// ShouldRedact reports whether the value at e must be replaced with the
// redaction sentinel. Delegates to types.ShouldRedact with the entry's
// schema-sensitivity flag and path pre-computed.
func ShouldRedact(e api.Entry, includeSensitive bool, sps types.SensitivePathChecker) bool {
	return types.ShouldRedact(includeSensitive, e.GetSchema(), e.SdcpbPath(), sps)
}

// RenderOpts holds common options shared across all northbound render operations.
type RenderOpts struct {
	OnlyNewOrUpdated bool
	IncludeSensitive  bool
	// SensitivePathSet is the union of sensitive_paths from all in-scope intents.
	// A nil value means no path-marker-based redaction is applied.
	// Accepts any types.SensitivePathChecker — e.g. a *types.SensitivePathIndex
	// in snapshot mode (Add) or the live datastore index (Set/Delete).
	SensitivePathSet types.SensitivePathChecker
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
