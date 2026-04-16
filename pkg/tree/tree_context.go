package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
)

type TreeContext struct {
	schemaClient     schemaClient.SchemaClientBound
	nonRevertiveInfo api.NonRevertiveInfos
	explicitDeletes  *api.DeletePathSet
	poolFactory      pool.VirtualPoolFactory
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		schemaClient:     sc,
		nonRevertiveInfo: api.NewNonRevertiveInfos(),
		explicitDeletes:  api.NewDeletePaths(),
		poolFactory:      poolFactory,
	}
}

// deepCopy root is required to be set manually
func (t *TreeContext) DeepCopy() api.TreeContext {
	tc := &TreeContext{
		schemaClient: t.schemaClient,
		poolFactory:  t.poolFactory,
	}

	tc.nonRevertiveInfo = t.nonRevertiveInfo.DeepCopy()
	tc.explicitDeletes = t.explicitDeletes.DeepCopy()
	return tc
}

func (t *TreeContext) PoolFactory() pool.VirtualPoolFactory {
	return t.poolFactory
}

func (t *TreeContext) SchemaClient() schemaClient.SchemaClientBound {
	return t.schemaClient
}

func (t *TreeContext) ExplicitDeletes() *api.DeletePathSet {
	return t.explicitDeletes
}

func (t *TreeContext) NonRevertiveInfo() api.NonRevertiveInfos {
	return t.nonRevertiveInfo
}
