package validation

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

func validateLength(_ context.Context, e api.Entry, resultChan chan<- *types.ValidationResultEntry, stats *types.ValidationStats) {
	if schema := e.GetSchema().GetField(); schema != nil {

		if len(schema.GetType().GetLength()) == 0 {
			return
		}

		lv := e.GetLeafVariants().GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		value := lv.Value().GetStringVal()
		actualLength := utf8.RuneCountInString(value)

		for _, lengthDef := range schema.GetType().GetLength() {

			if lengthDef.Min.Value <= uint64(actualLength) && uint64(actualLength) <= lengthDef.Max.Value {
				// continue if the length is within the range
				continue
			}
			// this is already the failure case
			lenghts := []string{}
			for _, lengthDef := range schema.GetType().GetLength() {
				lenghts = append(lenghts, fmt.Sprintf("%d..%d", lengthDef.Min.Value, lengthDef.Max.Value))
			}
			resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("error length of Path: %s, Value: %s not within allowed length %s", e.SdcpbPath().ToXPath(false), value, strings.Join(lenghts, ", ")), types.ValidationResultEntryTypeError)
		}
		stats.Add(types.StatTypeLength, uint32(len(schema.GetType().GetLength())))
	}
}
