package ops

import (
	"math"

	"github.com/sdcio/data-server/pkg/tree/api"
)

// GetHighestPrecedenceValueOfBranch goes through all the child branches to find the highest
// precedence value (lowest priority value) for the entire branch and returns it.
func GetHighestPrecedenceValueOfBranch(e api.Entry, filter api.HighestPrecedenceFilter) int32 {
	highestPrecedence := int32(math.MaxInt32)

	// Check all child branches (zero-allocation iteration)
	e.GetChildMap().ForEach(func(_ string, child api.Entry) {
		if childPrecedence := GetHighestPrecedenceValueOfBranch(child, filter); childPrecedence < highestPrecedence {
			highestPrecedence = childPrecedence
		}
	})

	// Check leaf variants
	if val := e.GetLeafVariants().GetHighestPrecedenceValue(filter); val < highestPrecedence {
		highestPrecedence = val
	}

	return highestPrecedence
}
