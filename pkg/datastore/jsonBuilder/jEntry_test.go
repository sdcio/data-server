package jsonbuilder

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestJEntry_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jEntry  *JEntry
		want    string
		wantErr bool
	}{
		{
			name: "one",
			jEntry: &JEntry{
				etype: ETMap,
				mapVal: map[string]*JEntry{
					"one": {
						etype:     ETString,
						stringVal: "onetwo",
					},
					"two": {
						etype: ETArray,
						arrayVal: []*JEntry{
							{
								etype:     ETString,
								stringVal: "Aonetwo",
							},
							{
								etype:     ETString,
								stringVal: "Atwotwo",
							},
						},
					},
				},
			},
			want: `{
  "one": "onetwo",
  "two": [
    "Aonetwo",
    "Atwotwo"
  ]
}`,
		},
		{
			name: "two",
			jEntry: &JEntry{
				etype: ETMap,
				mapVal: map[string]*JEntry{
					"one": {
						etype:     ETString,
						stringVal: "onetwo",
					},
					"two": {
						etype: ETMap,
						mapVal: map[string]*JEntry{
							"MEOne": NewJEntryString("one"),
							"METwo": NewJEntryString("two"),
						},
					},
				},
			},
			want: `{
  "one": "onetwo",
  "two": {
    "MEOne": "one",
    "METwo": "two"
  }
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := tt.jEntry
			got, err := json.MarshalIndent(j, "", "  ")
			if (err != nil) != tt.wantErr {
				t.Errorf("JEntry.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(string(got), tt.want); diff != "" {
				t.Errorf("JEntry.MarshalJSON() = %v, want %v, diff %s", string(got), tt.want, diff)
			}
		})
	}
}
