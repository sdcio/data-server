package validation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
)

// yangPatternToGo translates basic YANG (XML Schema) regex to Go (RE2) regex.
// - Removes ^ and $ anchors if present at start/end (YANG patterns are implicitly anchored)
// - Replaces \A/\Z with ^/$
// - Replaces \i and \c with best-effort character classes
// - Removes unsupported constructs (best effort, not comprehensive)
func yangPatternToGo(p string) string {
	// 1. Handle explicit anchors if they were manually added
	p = strings.TrimPrefix(p, "^")
	p = strings.TrimSuffix(p, "$")

	// 2. Replace XML-specific shortcuts
	// Note: Using raw strings `` to avoid double-backslash headache
	p = strings.ReplaceAll(p, `\i`, `[A-Za-z_]`)
	p = strings.ReplaceAll(p, `\c`, `[A-Za-z0-9._-]`)

	// 3. Translate \A and \Z just in case they were used
	p = strings.ReplaceAll(p, `\A`, `^`)
	p = strings.ReplaceAll(p, `\Z`, `$`)

	// 4. Force Implicit Anchoring (The YANG way)
	// We wrap in a non-capturing group to ensure | (OR) operators
	// don't break the anchors.
	return "^(?:" + p + ")$"
}

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
				goPattern := yangPatternToGo(p)
				matched, err := regexp.MatchString(goPattern, value)
				if err != nil {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("failed compiling regex (schema: %s, goPattern: %s) defined for %s", p, goPattern, e.SdcpbPath().ToXPath(false)), types.ValidationResultEntryTypeError)
					continue
				}
				if (!matched && !pattern.Inverted) || (pattern.GetInverted() && matched) {
					resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("value %s of %s does not match regex (schema: %s, goPattern: %s, inverted: %t)", value, e.SdcpbPath().ToXPath(false), p, goPattern, pattern.GetInverted()), types.ValidationResultEntryTypeError)
				}
			}

		}
		stats.Add(types.StatTypePattern, uint32(len(schema.GetType().GetPatterns())))
	}
}
