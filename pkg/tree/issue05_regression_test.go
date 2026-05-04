package tree

import (
	"context"
	"testing"
)

// Regression gate for Issue 05: end-to-end split semantics through RootEntry.DeepCopy.
// These tests verify the behavioral invariants that the TreeConfig / OperationState
// split must uphold across the import, transaction, and branch-copy flows.

// Behavior 1: Mutation of the copy's ExplicitDeletes does not affect the original.
func TestRootEntry_DeepCopy_CopyMutationDoesNotAffectOriginal(t *testing.T) {
	tc := newTestTreeContext(t)
	root, err := NewTreeRoot(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := root.DeepCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the copy's ExplicitDeletes.
	copied.GetTreeContext().GetOperationState().ExplicitDeletes().Add("copyOwner", 5, nil)

	// Original must still have zero entries under that intent.
	paths := root.GetTreeContext().GetOperationState().ExplicitDeletes().GetByIntentName("copyOwner")
	count := 0
	for range paths.Items() {
		count++
	}
	if count != 0 {
		t.Fatalf("expected original ExplicitDeletes to be unaffected after copy mutation, got %d path(s)", count)
	}
}

// Behavior 2: Mutation of the original's ExplicitDeletes after the copy does not affect the copy.
func TestRootEntry_DeepCopy_OriginalMutationPostCopyDoesNotAffectCopy(t *testing.T) {
	tc := newTestTreeContext(t)
	root, err := NewTreeRoot(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := root.DeepCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the original after the copy was taken.
	root.GetTreeContext().GetOperationState().ExplicitDeletes().Add("origOwner", 5, nil)

	// Copy must still have zero entries under that intent.
	paths := copied.GetTreeContext().GetOperationState().ExplicitDeletes().GetByIntentName("origOwner")
	count := 0
	for range paths.Items() {
		count++
	}
	if count != 0 {
		t.Fatalf("expected copy ExplicitDeletes to be unaffected by post-copy original mutation, got %d path(s)", count)
	}
}

// Behavior 3: Original and copy share the same TreeConfig pointer (immutable config identity).
func TestRootEntry_DeepCopy_SharesTreeConfigPointer(t *testing.T) {
	tc := newTestTreeContext(t)
	root, err := NewTreeRoot(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := root.DeepCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if root.GetTreeContext().GetTreeConfig() != copied.GetTreeContext().GetTreeConfig() {
		t.Fatal("expected original and copy to share the same TreeConfig instance (pointer identity)")
	}
}

// Behavior 4: Pre-copy NonRevertiveInfo is preserved in the copy, but subsequent
// mutations to the copy do not bleed back to the original.
func TestRootEntry_DeepCopy_NonRevertiveInfoIsDeepCopied(t *testing.T) {
	tc := newTestTreeContext(t)
	root, err := NewTreeRoot(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	// Record a NonRevertiveInfo entry on the original before copying.
	root.GetTreeContext().GetOperationState().NonRevertiveInfo().Add("ownerA", true)

	copied, err := root.DeepCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The copy should carry the pre-copy state.
	if !copied.GetTreeContext().GetOperationState().NonRevertiveInfo().IsGenerallyNonRevertive("ownerA") {
		t.Fatal("expected copy to preserve pre-copy NonRevertiveInfo entry")
	}

	// Adding a new entry to the copy must not affect the original.
	copied.GetTreeContext().GetOperationState().NonRevertiveInfo().Add("ownerCopyOnly", true)
	if root.GetTreeContext().GetOperationState().NonRevertiveInfo().IsGenerallyNonRevertive("ownerCopyOnly") {
		t.Fatal("expected original NonRevertiveInfo to be unaffected by copy-side mutation")
	}
}
