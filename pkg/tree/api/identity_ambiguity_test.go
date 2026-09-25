package api_test

import (
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree/api"
)

func TestValidateBareIdentityAtAmbiguousRoot(t *testing.T) {
	reg := schema.RootAmbiguityRegistry{
		{LocalName: "router", Modules: []string{"mod-a", "mod-b"}},
	}
	root := &stubRootEntry{}

	err := api.ValidateBareIdentityAtAmbiguousRoot(root, api.LocalIdentity("router"), reg)
	if err == nil {
		t.Fatal("expected error for bare router at ambiguous root")
	}
	if !strings.Contains(err.Error(), "mod-a") {
		t.Fatalf("error should list modules: %v", err)
	}

	if err := api.ValidateBareIdentityAtAmbiguousRoot(root, api.NodeIdentity{Local: "router", Module: "mod-a"}, reg); err != nil {
		t.Fatalf("qualified identity should be allowed: %v", err)
	}
	if err := api.ValidateBareIdentityAtAmbiguousRoot(root, api.LocalIdentity("interfaces"), reg); err != nil {
		t.Fatalf("unambiguous name should be allowed: %v", err)
	}
}

type stubRootEntry struct{}

func (stubRootEntry) IsRoot() bool { return true }
