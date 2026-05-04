package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
)

// treeConfig holds the immutable setup shared across all DeepCopy calls.
type treeConfig struct {
	schemaClient schemaClient.SchemaClientBound
	poolFactory  pool.VirtualPoolFactory
}

func (c *treeConfig) SchemaClient() schemaClient.SchemaClientBound {
	return c.schemaClient
}

func (c *treeConfig) PoolFactory() pool.VirtualPoolFactory {
	return c.poolFactory
}

type TreeContext struct {
	config         *treeConfig
	operationState api.OperationState
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		config: &treeConfig{
			schemaClient: sc,
			poolFactory:  poolFactory,
		},
		operationState: api.NewOperationState(),
	}
}

// GetTreeConfig returns the immutable tree setup. The same instance is shared
// across all DeepCopy calls.
func (t *TreeContext) GetTreeConfig() api.TreeConfig {
	return t.config
}

// GetOperationState returns the mutable per-operation state.
func (t *TreeContext) GetOperationState() api.OperationState {
	return t.operationState
}

// deepCopy root is required to be set manually
func (t *TreeContext) DeepCopy() api.TreeContext {
	return &TreeContext{
		config:         t.config, // shared by identity — immutable
		operationState: t.operationState.DeepCopyState(),
	}
}
