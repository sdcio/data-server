package validation

import (
	"context"
	"fmt"
	"regexp"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

func validatePattern(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetField(); schema != nil {
		if len(schema.GetType().GetPatterns()) == 0 {
			return
		}

		lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		for _, pattern := range schema.GetType().GetPatterns() {
			if p := pattern.GetPattern(); p != "" {
				matched, err := regexp.MatchString(p, value)
				if err != nil {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("failed compiling regex %s defined for %s", p, e.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("value %s of %s does not match regex %s (inverted: %t)", value, e.SdcpbPath().ToXPath(false), p, pattern.GetInverted()), types.ValidationResultEntryTypeError)
				}
			}

		}
		stats.Add(types.StatTypePattern, uint32(len(schema.GetType().GetPatterns())))
	}
}
