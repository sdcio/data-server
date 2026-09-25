package schema

import (
	"testing"

	ssSchema "github.com/sdcio/schema-server/pkg/schema"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestParseAmbiguityRegistryExclude(t *testing.T) {
	entries := []string{
		ssSchema.AmbiguousNameRegistryExcludePrefix + "/router=mod-a,mod-b,mod-c",
		"ietf-yang-library:other-exclude",
	}
	amb := ParseAmbiguityRegistryExclude(entries)
	if len(amb) != 1 {
		t.Fatalf("expected 1 ambiguity, got %d", len(amb))
	}
	if amb[0].LocalName != "router" || len(amb[0].Modules) != 3 {
		t.Fatalf("unexpected ambiguity: %+v", amb[0])
	}
	mods := ModulesForAmbiguousRootLocal(amb, "router")
	if len(mods) != 3 || mods[0] != "mod-a" {
		t.Fatalf("ModulesForAmbiguousRootLocal: %v", mods)
	}
	if ModulesForAmbiguousRootLocal(amb, "interfaces") != nil {
		t.Fatal("expected nil for unknown local name")
	}
}

func TestRootAmbiguitiesFromDetails(t *testing.T) {
	details := &sdcpb.GetSchemaDetailsResponse{
		Exclude: []string{
			ssSchema.AmbiguousNameRegistryExcludePrefix + "/router=only",
		},
	}
	amb := RootAmbiguitiesFromDetails(details)
	if len(amb) != 1 || amb[0].LocalName != "router" {
		t.Fatalf("unexpected: %+v", amb)
	}
	if RootAmbiguitiesFromDetails(nil) != nil {
		t.Fatal("nil details should return nil")
	}
}
