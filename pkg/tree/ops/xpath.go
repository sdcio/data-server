package ops

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/sdc-protos/sdcpb"
)

// ToXPath converts the branch under e to a list of XPath entries. It returns only the highest precedence value for each path.
func ToXPath(_ context.Context, e api.Entry, opts XPathRenderOpts) (*sdcpb.PathValues, error) {
	lvs := GetHighestPrecedence(e, opts.OnlyNewOrUpdated, opts.IncludeDefaults, false)

	xpaths := make([]*sdcpb.PathValue, 0, len(lvs))

	for _, lv := range lvs {
		value := lv.Value()
		if ShouldRedact(lv.GetEntry(), opts.IncludeSensitive, opts.SensitivePathSet) {
			value = types.RedactedTypedValue
		}
		xpaths = append(xpaths, &sdcpb.PathValue{
			Path:  lv.SdcpbPath(),
			Value: value,
		})
	}
	return &sdcpb.PathValues{PathValues: xpaths}, nil
}
