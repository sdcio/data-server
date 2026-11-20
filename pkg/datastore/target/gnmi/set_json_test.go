package gnmi

import (
	"encoding/json"
	"strings"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestJsonUpdatesPerTopLevelElement(t *testing.T) {
	type want struct {
		path  string // the update path's element names, "/"-joined; "" is the root path
		value string // its JSON value
	}
	tests := []struct {
		name string
		in   any
		want []want
	}{
		{
			name: "one update per top-level element, sorted, none on the root path",
			in: map[string]any{
				"System": map[string]any{"time-items": map[string]any{"adminSt": "enabled"}},
				"Other":  map[string]any{"a": 1.0},
			},
			want: []want{
				{path: "Other", value: `{"a":1}`},
				{path: "System", value: `{"time-items":{"adminSt":"enabled"}}`},
			},
		},
		{
			name: "a list element keeps its JSON array",
			in:   map[string]any{"interface": []any{map[string]any{"name": "e1"}}},
			want: []want{{path: "interface", value: `[{"name":"e1"}]`}},
		},
		{
			name: "empty object stays one update on the root path",
			in:   map[string]any{},
			want: []want{{path: "", value: `{}`}},
		},
		{
			name: "non-object stays one update on the root path",
			in:   []any{1.0, 2.0},
			want: []want{{path: "", value: `[1,2]`}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonUpdatesPerTopLevelElement(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d updates, want %d: %v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				names := []string{}
				for _, e := range got[i].GetPath().GetElem() {
					names = append(names, e.GetName())
				}
				if p := strings.Join(names, "/"); p != w.path {
					t.Errorf("update %d: path %q, want %q", i, p, w.path)
				}
				jv, ok := got[i].GetValue().GetValue().(*sdcpb.TypedValue_JsonVal)
				if !ok {
					t.Fatalf("update %d: value is %T, want a JSON value", i, got[i].GetValue().GetValue())
				}
				var gotV, wantV any
				if err := json.Unmarshal(jv.JsonVal, &gotV); err != nil {
					t.Fatalf("update %d: invalid JSON %q: %v", i, jv.JsonVal, err)
				}
				_ = json.Unmarshal([]byte(w.value), &wantV)
				gb, _ := json.Marshal(gotV)
				wb, _ := json.Marshal(wantV)
				if string(gb) != string(wb) {
					t.Errorf("update %d: value %s, want %s", i, gb, wb)
				}
			}
		})
	}
}
