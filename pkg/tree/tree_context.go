package tree

import (
	"fmt"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TreeContext struct {
	root             Entry // the trees root element
	schemaClient     schemaClient.SchemaClientBound
	actualOwner      string
	nonRevertiveInfo map[string]bool
	explicitDeletes  *DeletePathSet
	poolFactory      pool.VirtualPoolFactory
}

func NewTreeContext(sc schemaClient.SchemaClientBound, actualOwner string, poolFactory pool.VirtualPoolFactory) *TreeContext {
	return &TreeContext{
		schemaClient:     sc,
		actualOwner:      actualOwner,
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
	m := map[string]bool{}
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

func (t *TreeContext) SetRoot(e Entry) error {
	if t.root != nil {
		return fmt.Errorf("trying to set treecontexts root, although it is already set")
	}
	t.root = e
	return nil
}

func (t *TreeContext) GetActualOwner() string {
	return t.actualOwner
}

func (t *TreeContext) SetActualOwner(owner string) {
	t.actualOwner = owner
}
