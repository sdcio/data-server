package config

import (
	"testing"

	"github.com/sdcio/data-server/pkg/api"
	"gopkg.in/yaml.v2"
)

func gnmiSBI(enc api.GNMIEncoding) *SBI {
	return &SBI{Type: api.TargetTypeGNMI, Address: "a", Port: 1, GnmiOptions: &SBIGnmiOptions{Encoding: enc}}
}

func TestSyncEncodingInheritsFromTarget(t *testing.T) {
	ds := &DatastoreConfig{
		SBI:  gnmiSBI("json_ietf"),
		Sync: &Sync{Config: []*SyncProtocol{{Name: "a", Protocol: "gnmi"}, {Name: "b", Protocol: "gnmi", Encoding: "proto"}}},
	}
	if err := ds.ValidateSetDefaults(); err != nil {
		t.Fatal(err)
	}
	if got := ds.Sync.Config[0].Encoding; got != "JSON_IETF" {
		t.Errorf("inherited encoding = %q", got)
	}
	if got := ds.Sync.Config[1].Encoding; got != "PROTO" {
		t.Errorf("explicit encoding = %q", got)
	}
}

func TestSyncEncodingErrors(t *testing.T) {
	tests := map[string]*DatastoreConfig{
		"unknown target encoding": {SBI: gnmiSBI("jsno")},
		"numeric sync encoding":   {SBI: gnmiSBI("JSON"), Sync: &Sync{Config: []*SyncProtocol{{Name: "a", Protocol: "gnmi", Encoding: "4"}}}},
		"nothing to inherit":      {SBI: &SBI{Type: api.TargetTypeNetconf, Address: "a", Port: 1, NetconfOptions: &SBINetconfOptions{}}, Sync: &Sync{Config: []*SyncProtocol{{Name: "a", Protocol: "gnmi"}}}},
	}
	for name, ds := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ds.ValidateSetDefaults(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTypedFieldsDecodeAndReject(t *testing.T) {
	var sp SyncProtocol
	if err := yaml.Unmarshal([]byte("protocol: GNMI\nmode: Sample\nencoding: json_ietf\n"), &sp); err != nil {
		t.Fatal(err)
	}
	if sp.Protocol != api.TargetTypeGNMI || sp.Mode != SyncModeSample || sp.Encoding != api.GNMIEncodingJSONIETF {
		t.Errorf("unexpected decode: %+v", sp)
	}
	if err := yaml.Unmarshal([]byte("mode: sometimes\n"), &sp); err == nil {
		t.Error("unknown mode accepted")
	}
	if err := (&CacheConfig{Type: "bogus"}).validateSetDefaults(); err == nil {
		t.Error("unknown cache type accepted")
	}
	if err := (&SBINetconfOptions{PreferredNCVersion: "2.0"}).validateSetDefaults(); err == nil {
		t.Error("unknown netconf version accepted")
	}
}
