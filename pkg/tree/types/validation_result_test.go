package types

import (
	"errors"
	"testing"
)

// TestValidationResults_HasErrorsExcludingOwners covers ticket 05's safety
// net: errors owned by an excluded intent name must not count towards
// HasErrorsExcludingOwners, while errors owned by any non-excluded intent
// still do.
func TestValidationResults_HasErrorsExcludingOwners(t *testing.T) {
	tests := []struct {
		name          string
		results       ValidationResults
		excludeOwners map[string]struct{}
		wantHasErrors bool
	}{
		{
			name:          "no results",
			results:       ValidationResults{},
			excludeOwners: map[string]struct{}{},
			wantHasErrors: false,
		},
		{
			name: "error owned by excluded intent only",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("ghost", errors.New("dangling leafref"), ValidationResultEntryTypeError))
				return v
			}(),
			excludeOwners: map[string]struct{}{"ghost": {}},
			wantHasErrors: false,
		},
		{
			name: "error owned by non-excluded intent",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("customer", errors.New("mandatory leaf missing"), ValidationResultEntryTypeError))
				return v
			}(),
			excludeOwners: map[string]struct{}{"ghost": {}},
			wantHasErrors: true,
		},
		{
			name: "errors on both excluded and non-excluded intents",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("ghost", errors.New("dangling leafref"), ValidationResultEntryTypeError))
				_ = v.AddEntry(NewValidationResultEntry("customer", errors.New("mandatory leaf missing"), ValidationResultEntryTypeError))
				return v
			}(),
			excludeOwners: map[string]struct{}{"ghost": {}},
			wantHasErrors: true,
		},
		{
			name: "nil exclude set behaves like HasErrors",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("intent1", errors.New("boom"), ValidationResultEntryTypeError))
				return v
			}(),
			excludeOwners: nil,
			wantHasErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.results.HasErrorsExcludingOwners(tt.excludeOwners); got != tt.wantHasErrors {
				t.Errorf("HasErrorsExcludingOwners() = %v, want %v", got, tt.wantHasErrors)
			}
		})
	}
}
