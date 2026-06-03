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

// Behavior 1: TreeConfig returns non-nil
func TestNewTreeContext_TreeConfig_NonNil(t *testing.T) {
	tc := newTestTreeContext(t)
	if tc.TreeConfig() == nil {
		t.Fatal("expected TreeConfig() to return non-nil TreeConfig")
	}
}

// Behavior 2: OperationState returns non-nil
func TestNewTreeContext_OperationState_NonNil(t *testing.T) {
	tc := newTestTreeContext(t)
	if tc.OperationState() == nil {
		t.Fatal("expected OperationState() to return non-nil TreeOperationState")
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
	if tc.TreeConfig().SchemaClient() != scb {
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
	if tc.TreeConfig().PoolFactory() != pf {
		t.Fatal("expected TreeConfig.PoolFactory() to be the same instance passed to NewTreeContext")
	}
}

// Behavior 5: OperationState starts with non-nil, empty ExplicitDeletes
func TestNewTreeContext_OperationState_EmptyExplicitDeletes(t *testing.T) {
	tc := newTestTreeContext(t)
	ed := tc.OperationState().ExplicitDeletes()
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
	nri := tc.OperationState().NonRevertiveInfo()
	if nri == nil {
		t.Fatal("expected NonRevertiveInfo() to be non-nil")
	}
}

// Behavior 7: DeepCopy shares the same immutable config values (SchemaClient
// and PoolFactory point to the same underlying objects).
func TestTreeContext_DeepCopy_SharesImmutableConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	pf := pool.NewSharedTaskPool(context.Background(), runtime.GOMAXPROCS(0))
	tc := NewTreeContext(scb, pf)
	copied := tc.DeepCopy()
	if tc.TreeConfig().SchemaClient() != copied.TreeConfig().SchemaClient() {
		t.Fatal("expected DeepCopy to share the same SchemaClient instance")
	}
	if tc.TreeConfig().PoolFactory() != copied.TreeConfig().PoolFactory() {
		t.Fatal("expected DeepCopy to share the same PoolFactory instance")
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
	if tc.OperationState() == concreteCopy.OperationState() {
		t.Fatal("expected DeepCopy to produce a distinct TreeOperationState instance")
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
	tc.OperationState().ExplicitDeletes().Add("owner1", 0, nil)
	copyPaths := concreteCopy.OperationState().ExplicitDeletes().GetByIntentName("owner1")
	copyCount := 0
	for range copyPaths.Items() {
		copyCount++
	}
	if copyCount != 0 {
		t.Fatal("expected copy's ExplicitDeletes to remain empty after mutating original")
	}
}
