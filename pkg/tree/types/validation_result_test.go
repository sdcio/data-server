package types

import (
	"errors"
	"testing"
)

func TestValidationResults_HasErrors(t *testing.T) {
	tests := []struct {
		name          string
		results       ValidationResults
		wantHasErrors bool
	}{
		{
			name:          "empty results",
			results:       ValidationResults{},
			wantHasErrors: false,
		},
		{
			name: "single intent with error",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("intent1", errors.New("mandatory leaf missing"), ValidationResultEntryTypeError))
				return v
			}(),
			wantHasErrors: true,
		},
		{
			name: "single intent with warning only — not an error",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("intent1", errors.New("deviation observed"), ValidationResultEntryTypeWarning))
				return v
			}(),
			wantHasErrors: false,
		},
		{
			name: "multiple intents all with errors",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry("intent1", errors.New("leafref broken"), ValidationResultEntryTypeError))
				_ = v.AddEntry(NewValidationResultEntry("intent2", errors.New("mandatory missing"), ValidationResultEntryTypeError))
				return v
			}(),
			wantHasErrors: true,
		},
		{
			name: "unknown-owner error blocks — not special-cased",
			results: func() ValidationResults {
				v := ValidationResults{}
				_ = v.AddEntry(NewValidationResultEntry(UnknownOwner, errors.New("mandatory child [autonomous-system] does not exist"), ValidationResultEntryTypeError))
				return v
			}(),
			wantHasErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.results.HasErrors(); got != tt.wantHasErrors {
				t.Errorf("HasErrors() = %v, want %v", got, tt.wantHasErrors)
			}
		})
	}
}
