package config

import "testing"

func gnmiSBI(enc string) *SBI {
	return &SBI{Type: sbiGNMI, Address: "a", Port: 1, GnmiOptions: &SBIGnmiOptions{Encoding: enc}}
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
		"nothing to inherit":      {SBI: &SBI{Type: sbiNETCONF, Address: "a", Port: 1, NetconfOptions: &SBINetconfOptions{}}, Sync: &Sync{Config: []*SyncProtocol{{Name: "a", Protocol: "gnmi"}}}},
	}
	for name, ds := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ds.ValidateSetDefaults(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
