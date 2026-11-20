package gnmi

import (
	"encoding/json"
	"maps"
	"slices"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// jsonUpdatesPerTopLevelElement turns the JSON of a Set into one update per top-level element
// (path /<name>, the element's JSON as value) instead of a single update on the root path.
// Some devices refuse updates on the root path: Cisco NX-OS answers "root path is not allowed",
// and it accepts no other value type than JSON, so the per-leaf updates of the proto encoding do
// not help either ("unsupported value type in operation").
// Anything that is not a non-empty JSON object is sent on the root path, as before.
func jsonUpdatesPerTopLevelElement(jsonData any) ([]*sdcpb.Update, error) {
	obj, ok := jsonData.(map[string]any)
	if !ok || len(obj) == 0 {
		b, err := json.Marshal(jsonData)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			return nil, nil
		}
		return []*sdcpb.Update{jsonUpdate(&sdcpb.Path{}, b)}, nil
	}

	upds := make([]*sdcpb.Update, 0, len(obj))
	for _, name := range slices.Sorted(maps.Keys(obj)) {
		b, err := json.Marshal(obj[name])
		if err != nil {
			return nil, err
		}
		upds = append(upds, jsonUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: name}}}, b))
	}
	return upds, nil
}

func jsonUpdate(p *sdcpb.Path, b []byte) *sdcpb.Update {
	return &sdcpb.Update{Path: p, Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: b}}}
}
