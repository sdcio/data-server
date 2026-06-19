package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
)

// TreeContext holds immutable setup fields (SchemaClient, PoolFactory) and
// mutable per-operation state (TreeOperationState). It implements both
// api.TreeContext and api.TreeConfig: TreeConfig() returns itself, so callers
// reading immutable fields see a narrower interface that signals "this never
// changes," while callers reading mutable state use OperationState().
type TreeContext struct {
	schemaClient   schemaClient.SchemaClientBound
	poolFactory    pool.VirtualPoolFactory
	operationState api.TreeOperationState
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		schemaClient:   sc,
		poolFactory:    poolFactory,
		operationState: api.NewTreeOperationState(),
	}
}

// TreeConfig implements api.TreeContext. Returns self — the immutable fields
// live directly on TreeContext, so no wrapper is needed.
func (t *TreeContext) TreeConfig() api.TreeConfig {
	return t
}

// OperationState implements api.TreeContext.
func (t *TreeContext) OperationState() api.TreeOperationState {
	return t.operationState
}

// SchemaClient implements api.TreeConfig.
func (t *TreeContext) SchemaClient() schemaClient.SchemaClientBound {
	return t.schemaClient
}

// PoolFactory implements api.TreeConfig.
func (t *TreeContext) PoolFactory() pool.VirtualPoolFactory {
	return t.poolFactory
}

// DeepCopy implements api.TreeContext. Interface values (schemaClient,
// poolFactory) copy as references — the underlying objects are shared, which
// is the correct "immutable config" semantics. Only operationState is
// deep-copied.
func (t *TreeContext) DeepCopy() api.TreeContext {
	return &TreeContext{
		schemaClient:   t.schemaClient,
		poolFactory:    t.poolFactory,
		operationState: t.operationState.DeepCopy(),
	}
}
