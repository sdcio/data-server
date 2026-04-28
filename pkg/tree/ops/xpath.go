package ops

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/sdc-protos/sdcpb"
)

// ToXPath converts the branch under e to a list of XPath entries. It returns only the highest precedence value for each path.
func ToXPath(_ context.Context, e api.Entry, onlyNewOrUpdated, includeDefaults bool) (*sdcpb.PathValues, error) {
	lvs := GetHighestPrecedence(e, onlyNewOrUpdated, includeDefaults, false)

	xpaths := make([]*sdcpb.PathValue, 0, len(lvs))

	for _, lv := range lvs {
		xpaths = append(xpaths, &sdcpb.PathValue{
			Path:  lv.SdcpbPath(),
			Value: lv.Value(),
		})
	}
	return &sdcpb.PathValues{PathValues: xpaths}, nil
}
