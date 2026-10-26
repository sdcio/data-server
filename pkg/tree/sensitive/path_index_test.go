package sensitive_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/sensitive"
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

func TestPathIndex_SetContains(t *testing.T) {
	idx := sensitive.NewPathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})

	if !idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = false after Set, want true")
	}
}

func TestPathIndex_DeleteRemovesIntentPaths(t *testing.T) {
	idx := sensitive.NewPathIndex()
	idx.Set("my-intent", []*sdcpb.Path{mustParsePath(t, "/ospf/auth-key")})
	idx.Delete("my-intent")

	if idx.Contains(mustParsePath(t, "/ospf/auth-key")) {
		t.Error("Contains() = true after Delete, want false")
	}
	if idx.PathsFor("my-intent") != nil {
		t.Error("PathsFor() after Delete should be nil")
	}
}

func TestPathIndex_PathsForReturnsSnapshot(t *testing.T) {
	idx := sensitive.NewPathIndex()
	path := mustParsePath(t, "/ospf/auth-key")
	idx.Set("my-intent", []*sdcpb.Path{path})

	got := idx.PathsFor("my-intent")
	if len(got) != 1 || got[0].ToXPath(true) != path.ToXPath(true) {
		t.Fatalf("PathsFor() = %v, want one path %q", got, path.ToXPath(true))
	}
	got[0] = mustParsePath(t, "/other")
	if idx.PathsFor("my-intent")[0].ToXPath(true) != path.ToXPath(true) {
		t.Error("PathsFor() must return a copy; mutating the slice must not change the index")
	}
}

func TestPathIndex_DeleteKeepsSharedPath(t *testing.T) {
	sharedPath := mustParsePath(t, "/ospf/auth-key")
	idx := sensitive.NewPathIndex()
	idx.Set("intent-a", []*sdcpb.Path{sharedPath})
	idx.Set("intent-b", []*sdcpb.Path{sharedPath})

	idx.Delete("intent-a")

	if !idx.Contains(sharedPath) {
		t.Error("Contains() = false after deleting one intent, want true (path still owned by intent-b)")
	}
}

func TestPathIndex_NilReceiverContains(t *testing.T) {
	var idx *sensitive.PathIndex
	if idx.Contains(mustParsePath(t, "/bgp/neighbors/auth-password")) {
		t.Error("Contains() on nil receiver = true, want false")
	}
}

func TestPathIndex_KeyPrunedMatch(t *testing.T) {
	t.Run("keyless stored, keyed lookup", func(t *testing.T) {
		idx := sensitive.NewPathIndex()
		idx.Set("intent", []*sdcpb.Path{mustParsePath(t, "/interface/secret")})

		if !idx.Contains(mustParsePath(t, "/interface[name=eth0]/secret")) {
			t.Error("Contains() = false for keyed lookup against keyless stored path, want true")
		}
	})

	t.Run("keyed stored, keyless lookup", func(t *testing.T) {
		idx := sensitive.NewPathIndex()
		idx.Set("intent", []*sdcpb.Path{mustParsePath(t, "/interface[name=eth0]/secret")})

		if !idx.Contains(mustParsePath(t, "/interface/secret")) {
			t.Error("Contains() = false for keyless lookup against keyed stored path, want true")
		}
	})
}
