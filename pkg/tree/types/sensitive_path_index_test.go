package types_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func mustParsePath(t *testing.T, s string) *sdcpb.Path {
	t.Helper()
	p, err := sdcpb.ParsePath(s)
	if err != nil {
		t.Fatalf("mustParsePath(%q): %v", s, err)
	}
	return p
}

func TestSensitivePaths_Contains(t *testing.T) {
	sps := types.NewSensitivePaths(mustParsePath(t, "/bgp/neighbors/auth-password"))

	if !sps.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() = false after NewSensitivePaths, want true")
	}
}

func TestSensitivePaths_ContainsUnknownPath(t *testing.T) {
	sps := types.NewSensitivePaths(mustParsePath(t, "/bgp/neighbors/auth-password"))

	if sps.Contains(mustParsePath(t, "/bgp/neighbors/other")) {
		t.Error("Contains() = true for unknown path, want false")
	}
}

func TestSensitivePaths_KeyPrunedMatch(t *testing.T) {
	t.Run("keyless stored, keyed lookup", func(t *testing.T) {
		sps := types.NewSensitivePaths(mustParsePath(t, "/interface/secret"))

		if !sps.Contains(mustParsePath(t, "/interface[name=eth0]/secret")) {
			t.Error("Contains() = false for keyed lookup against keyless stored path, want true")
		}
	})

	t.Run("keyed stored, keyless lookup", func(t *testing.T) {
		sps := types.NewSensitivePaths(mustParsePath(t, "/interface[name=eth0]/secret"))

		if !sps.Contains(mustParsePath(t, "/interface/secret")) {
			t.Error("Contains() = false for keyless lookup against keyed stored path, want true")
		}
	})
}

func TestSensitivePaths_NilReceiverContains(t *testing.T) {
	var sps *types.SensitivePaths
	if sps.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() on nil receiver = true, want false")
	}
}

func TestSensitivePathIndex_SetContains(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})

	if !idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = false after Set, want true")
	}
}

func TestSensitivePathIndex_DeleteRemovesIntentPaths(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})
	idx.Delete("my-intent")

	if idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = true after Delete, want false")
	}
}

func TestSensitivePathIndex_DeleteKeepsSharedPath(t *testing.T) {
	sharedPath := mustParsePath(t, "/ospf/auth-key")
	idx := types.NewSensitivePathIndex()
	idx.Set("intent-a", []*sdcpb.Path{sharedPath})
	idx.Set("intent-b", []*sdcpb.Path{sharedPath})

	idx.Delete("intent-a")

	if !idx.Contains(sharedPath) {
		t.Error("Contains() = false after deleting one intent, want true (path still owned by intent-b)")
	}
}

func TestSensitivePathIndex_NilReceiverContains(t *testing.T) {
	var idx *types.SensitivePathIndex
	if idx.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() on nil receiver = true, want false")
	}
}

func TestSensitivePathIndex_KeyPrunedMatch(t *testing.T) {
	t.Run("keyless stored, keyed lookup", func(t *testing.T) {
		idx := types.NewSensitivePathIndex()
		idx.Set("intent", []*sdcpb.Path{mustParsePath(t, "/interface/secret")})

		if !idx.Contains(mustParsePath(t, "/interface[name=eth0]/secret")) {
			t.Error("Contains() = false for keyed lookup against keyless stored path, want true")
		}
	})

	t.Run("keyed stored, keyless lookup", func(t *testing.T) {
		idx := types.NewSensitivePathIndex()
		idx.Set("intent", []*sdcpb.Path{mustParsePath(t, "/interface[name=eth0]/secret")})

		if !idx.Contains(mustParsePath(t, "/interface/secret")) {
			t.Error("Contains() = false for keyless lookup against keyed stored path, want true")
		}
	})
}
