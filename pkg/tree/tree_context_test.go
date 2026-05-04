package tree

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func newTestTreeContext(t *testing.T) *TreeContext {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	return NewTreeContext(scb, pool.NewSharedTaskPool(context.Background(), runtime.GOMAXPROCS(0)))
}

// Behavior 1: GetTreeConfig returns non-nil
func TestNewTreeContext_GetTreeConfig_NonNil(t *testing.T) {
	tc := newTestTreeContext(t)
	if tc.GetTreeConfig() == nil {
		t.Fatal("expected GetTreeConfig() to return non-nil TreeConfig")
	}
}

// Behavior 2: GetOperationState returns non-nil
func TestNewTreeContext_GetOperationState_NonNil(t *testing.T) {
	tc := newTestTreeContext(t)
	if tc.GetOperationState() == nil {
		t.Fatal("expected GetOperationState() to return non-nil OperationState")
	}
}

// Behavior 3: TreeConfig carries the schema client
func TestNewTreeContext_TreeConfig_SchemaClient(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.Background(), runtime.GOMAXPROCS(0)))
	if tc.GetTreeConfig().SchemaClient() != scb {
		t.Fatal("expected TreeConfig.SchemaClient() to be the same instance passed to NewTreeContext")
	}
}

// Behavior 4: TreeConfig carries the pool factory
func TestNewTreeContext_TreeConfig_PoolFactory(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	pf := pool.NewSharedTaskPool(context.Background(), runtime.GOMAXPROCS(0))
	tc := NewTreeContext(scb, pf)
	if tc.GetTreeConfig().PoolFactory() != pf {
		t.Fatal("expected TreeConfig.PoolFactory() to be the same instance passed to NewTreeContext")
	}
}

// Behavior 5: OperationState starts with non-nil, empty ExplicitDeletes
func TestNewTreeContext_OperationState_EmptyExplicitDeletes(t *testing.T) {
	tc := newTestTreeContext(t)
	ed := tc.GetOperationState().ExplicitDeletes()
	if ed == nil {
		t.Fatal("expected ExplicitDeletes() to be non-nil")
	}
	count := 0
	for range ed.Items() {
		count++
	}
	if count != 0 {
		t.Fatalf("expected ExplicitDeletes to be empty, got %d entries", count)
	}
}

// Behavior 6: OperationState starts with non-nil, empty NonRevertiveInfo
func TestNewTreeContext_OperationState_EmptyNonRevertiveInfo(t *testing.T) {
	tc := newTestTreeContext(t)
	nri := tc.GetOperationState().NonRevertiveInfo()
	if nri == nil {
		t.Fatal("expected NonRevertiveInfo() to be non-nil")
	}
}

// Behavior 7: DeepCopy reuses the same TreeConfig instance (pointer identity)
func TestTreeContext_DeepCopy_ReusesTreeConfig(t *testing.T) {
	tc := newTestTreeContext(t)
	copied := tc.DeepCopy()
	concreteCopy, ok := copied.(*TreeContext)
	if !ok {
		t.Fatal("DeepCopy did not return a *TreeContext")
	}
	if tc.GetTreeConfig() != concreteCopy.GetTreeConfig() {
		t.Fatal("expected DeepCopy to reuse the same TreeConfig instance (pointer identity)")
	}
}

// Behavior 8: DeepCopy produces a distinct OperationState instance
func TestTreeContext_DeepCopy_DistinctOperationState(t *testing.T) {
	tc := newTestTreeContext(t)
	copied := tc.DeepCopy()
	concreteCopy, ok := copied.(*TreeContext)
	if !ok {
		t.Fatal("DeepCopy did not return a *TreeContext")
	}
	if tc.GetOperationState() == concreteCopy.GetOperationState() {
		t.Fatal("expected DeepCopy to produce a distinct OperationState instance")
	}
}

// Behavior 9: Mutations to original OperationState do not affect the copy
func TestTreeContext_DeepCopy_MutationIsolation(t *testing.T) {
	tc := newTestTreeContext(t)
	copied := tc.DeepCopy()
	concreteCopy, ok := copied.(*TreeContext)
	if !ok {
		t.Fatal("DeepCopy did not return a *TreeContext")
	}
	// Add an explicit delete to the original; copy must stay empty
	tc.GetOperationState().ExplicitDeletes().Add("owner1", 0, nil)
	copyPaths := concreteCopy.GetOperationState().ExplicitDeletes().GetByIntentName("owner1")
	copyCount := 0
	for range copyPaths.Items() {
		copyCount++
	}
	if copyCount != 0 {
		t.Fatal("expected copy's ExplicitDeletes to remain empty after mutating original")
	}
}
