package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TreeContext struct {
	schemaClient     schemaClient.SchemaClientBound
	nonRevertiveInfo map[string]bool
	explicitDeletes  *DeletePathSet
	poolFactory      pool.VirtualPoolFactory
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		schemaClient:     sc,
		nonRevertiveInfo: map[string]bool{},
		explicitDeletes:  NewDeletePaths(),
		poolFactory:      poolFactory,
	}
}

// deepCopy root is required to be set manually
func (t *TreeContext) deepCopy() *TreeContext {
	tc := &TreeContext{
		schemaClient: t.schemaClient,
		poolFactory:  t.poolFactory,
	}

	// deepcopy nonRevertiveInfo
	m := make(map[string]bool, len(t.nonRevertiveInfo))
	for k, v := range t.nonRevertiveInfo {
		m[k] = v
	}
	tc.nonRevertiveInfo = m
	tc.explicitDeletes = t.explicitDeletes.DeepCopy()
	return tc
}

func (t *TreeContext) GetPoolFactory() pool.VirtualPoolFactory {
	return t.poolFactory
}

func (t *TreeContext) AddExplicitDeletes(intentName string, priority int32, pathset *sdcpb.PathSet) {
	t.explicitDeletes.Add(intentName, priority, pathset)
}

func (t *TreeContext) RemoveExplicitDeletes(intentName string) *sdcpb.PathSet {
	return t.explicitDeletes.RemoveIntentDeletes(intentName)
}

func (t *TreeContext) AddNonRevertiveInfo(intent string, nonRevertive bool) {
	t.nonRevertiveInfo[intent] = nonRevertive
}

// IsNonRevertiveIntent returns the non-revertive flag per intent. False is also returned the intent does not exist.
func (t *TreeContext) IsNonRevertiveIntent(intent string) bool {
	return t.nonRevertiveInfo[intent]
}
