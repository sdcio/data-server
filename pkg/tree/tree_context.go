package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/interfaces"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TreeContext struct {
	schemaClient     schemaClient.SchemaClientBound
	nonRevertiveInfo map[string]*NonRevertiveInfo
	explicitDeletes  *DeletePathSet
	poolFactory      pool.VirtualPoolFactory
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		schemaClient:     sc,
		nonRevertiveInfo: map[string]*NonRevertiveInfo{},
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
	tc.nonRevertiveInfo = make(map[string]*NonRevertiveInfo, len(t.nonRevertiveInfo))
	for intent, info := range t.nonRevertiveInfo {
		tc.nonRevertiveInfo[intent] = info.DeepCopy()
	}

	// deepcopy explicitDeletes
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
	t.nonRevertiveInfo[intent] = NewNonRevertiveInfo(intent, nonRevertive)
}

// IsNonRevertiveIntent returns true if the intent is non-revertive for all paths, false otherwise. If the intent does not exist, false is returned.
func (t *TreeContext) IsGenerallyNonRevertiveIntent(intent string) bool {
	if _, ok := t.nonRevertiveInfo[intent]; !ok {
		return false
	}
	return t.nonRevertiveInfo[intent].GetGeneralNonRevertiveState()
}

// IsNonRevertiveIntent returns the non-revertive flag per intent. False is also returned the intent does not exist.
func (t *TreeContext) IsNonRevertiveIntentPath(intent string, p interfaces.SdcpbPath) bool {
	if _, ok := t.nonRevertiveInfo[intent]; !ok {
		return false
	}
	return t.nonRevertiveInfo[intent].IsNonRevertive(p)
}

func (t *TreeContext) GetNonRevertiveInfo(intent string) (*NonRevertiveInfo, bool) {
	info, ok := t.nonRevertiveInfo[intent]
	return info, ok
}
