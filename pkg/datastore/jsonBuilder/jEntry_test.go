package jsonbuilder

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestJEntry_MarshalJSON(t *testing.T) {

	tests := []struct {
		name    string
		jEntry  *JEntry
		want    []byte
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := tt.jEntry
			got, err := json.MarshalIndent(j, "", "  ")
			fmt.Println(string(got))
			if (err != nil) != tt.wantErr {
				t.Errorf("JEntry.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JEntry.MarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
