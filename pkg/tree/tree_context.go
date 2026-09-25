package tree

import (
	"context"
	"sync"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree/api"
)

type TreeContext struct {
	schemaClient     schemaClient.SchemaClientBound
	nonRevertiveInfo api.NonRevertiveInfos
	explicitDeletes  *api.DeletePathSet
	poolFactory      pool.VirtualPoolFactory

	rootAmbiguityMu      sync.Mutex
	rootAmbiguity        schema.RootAmbiguityRegistry
	rootAmbiguityErr     error
	rootAmbiguityReady   bool
}

type rootAmbiguityRegistryLoader interface {
	RootAmbiguityRegistry(context.Context) (schema.RootAmbiguityRegistry, error)
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

	t.rootAmbiguityMu.Lock()
	tc.rootAmbiguity = t.rootAmbiguity
	tc.rootAmbiguityErr = t.rootAmbiguityErr
	tc.rootAmbiguityReady = t.rootAmbiguityReady
	t.rootAmbiguityMu.Unlock()

	return tc
}

func (t *TreeContext) PoolFactory() pool.VirtualPoolFactory {
	return t.poolFactory
}

func (t *TreeContext) SchemaClient() schemaClient.SchemaClientBound {
	return t.schemaClient
}

func (t *TreeContext) RootAmbiguityRegistry(ctx context.Context) (schema.RootAmbiguityRegistry, error) {
	t.rootAmbiguityMu.Lock()
	defer t.rootAmbiguityMu.Unlock()
	if t.rootAmbiguityReady {
		return t.rootAmbiguity, t.rootAmbiguityErr
	}
	loader, ok := t.schemaClient.(rootAmbiguityRegistryLoader)
	if !ok {
		t.rootAmbiguityReady = true
		return nil, nil
	}
	reg, err := loader.RootAmbiguityRegistry(ctx)
	t.rootAmbiguity = reg
	t.rootAmbiguityErr = err
	t.rootAmbiguityReady = true
	return reg, err
}

func (t *TreeContext) ExplicitDeletes() *api.DeletePathSet {
	return t.explicitDeletes
}

func (t *TreeContext) NonRevertiveInfo() api.NonRevertiveInfos {
	return t.nonRevertiveInfo
}
