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

// Cycle 1 — Add marks a path as sensitive; Contains returns true.
func TestSensitivePathIndex_AddContains(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Add(mustParsePath(t, "/bgp/neighbors/auth-password"))

	if !idx.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() = false after Add, want true")
	}
}

// Cycle 2 — Contains returns false for a path that was never added.
func TestSensitivePathIndex_ContainsUnknownPath(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Add(mustParsePath(t, "/bgp/neighbors/auth-password"))

	if idx.Contains(mustParsePath(t, "/bgp/neighbors/other")) {
		t.Error("Contains() = true for unknown path, want false")
	}
}

// Cycle 3 — key-pruned matching: keyed lookup matches keyless stored path and vice-versa.
func TestSensitivePathIndex_KeyPrunedMatch(t *testing.T) {
	t.Run("keyless stored, keyed lookup", func(t *testing.T) {
		idx := types.NewSensitivePathIndex()
		idx.Add(mustParsePath(t, "/interface/secret"))

		if !idx.Contains(mustParsePath(t, "/interface[name=eth0]/secret")) {
			t.Error("Contains() = false for keyed lookup against keyless stored path, want true")
		}
	})

	t.Run("keyed stored, keyless lookup", func(t *testing.T) {
		idx := types.NewSensitivePathIndex()
		idx.Add(mustParsePath(t, "/interface[name=eth0]/secret"))

		if !idx.Contains(mustParsePath(t, "/interface/secret")) {
			t.Error("Contains() = false for keyless lookup against keyed stored path, want true")
		}
	})
}

// Cycle 4 — Set(intentName, paths) makes paths discoverable via Contains.
func TestSensitivePathIndex_SetContains(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})

	if !idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = false after Set, want true")
	}
}

// Cycle 5 — Delete removes all paths contributed by that intent.
func TestSensitivePathIndex_DeleteRemovesIntentPaths(t *testing.T) {
	idx := types.NewSensitivePathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})
	idx.Delete("my-intent")

	if idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = true after Delete, want false")
	}
}

// Cycle 6 — Delete does not remove a path still contributed by another intent.
func TestSensitivePathIndex_DeleteKeepsSharedPath(t *testing.T) {
	sharedPath := mustParsePath(t, "/bgp/neighbors/auth-password")
	idx := types.NewSensitivePathIndex()
	idx.Set("intent-a", []*sdcpb.Path{sharedPath})
	idx.Set("intent-b", []*sdcpb.Path{sharedPath})

	idx.Delete("intent-a")

	if !idx.Contains(sharedPath) {
		t.Error("Contains() = false after deleting one intent, want true (path still owned by intent-b)")
	}
}

// Cycle 7 — nil receiver returns false without panicking.
func TestSensitivePathIndex_NilReceiverContains(t *testing.T) {
	var idx *types.SensitivePathIndex
	if idx.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() on nil receiver = true, want false")
	}
}
