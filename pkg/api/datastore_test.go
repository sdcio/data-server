package api

import "testing"

func TestParseGNMIEncoding(t *testing.T) {
	for in, want := range map[string]GNMIEncoding{"json_ietf": GNMIEncodingJSONIETF, "JSON": GNMIEncodingJSON, "Proto": GNMIEncodingProto, "config": GNMIEncodingConfig} {
		if got, err := ParseGNMIEncoding(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "jsno", "4"} {
		if _, err := ParseGNMIEncoding(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}
