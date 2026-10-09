package ops

import (
	"github.com/sdcio/data-server/pkg/tree/sensitive"
)

// RenderOpts holds common options shared across all render operations.
// Redaction is owned by the embedded sensitive.Render; build RenderOpts with
// RenderOptsNorthbound or RenderOptsRevealAll rather than setting it by hand.
type RenderOpts struct {
	OnlyNewOrUpdated bool
	sensitive.Render
}

// RenderOptsNorthbound builds opts for northbound APIs (GetIntent, BlameConfig,
// WatchDeviations). live is the Live Sensitive Path Index.
func RenderOptsNorthbound(includeSensitive bool, live sensitive.PathChecker) RenderOpts {
	return RenderOpts{Render: sensitive.NewRender(includeSensitive, live)}
}

// RenderOptsRevealAll builds opts for southbound emission (sync / device output)
// where secrets must pass through in plaintext.
func RenderOptsRevealAll() RenderOpts {
	return RenderOpts{Render: sensitive.RevealAll()}
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
