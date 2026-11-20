package tree

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TreeContextOption configures optional collaborators on a TreeContext.
type TreeContextOption func(*TreeContext)

// WithConfirmedAppliedValues wires a read-only baseline resolver for
// mutate-on-write leaves. When resolver also implements
// api.LearnedMutateOnWritePathSet, TreeContext queries it for dynamically
// learned mutate-on-write schema paths.
func WithConfirmedAppliedValues(resolver api.ConfirmedAppliedValueResolver) TreeContextOption {
	return func(tc *TreeContext) {
		tc.confirmedAppliedValues = resolver
		if learned, ok := resolver.(api.LearnedMutateOnWritePathSet); ok {
			tc.learnedMutateOnWritePaths = learned
		}
	}
}

type TreeContext struct {
	schemaClient              schemaClient.SchemaClientBound
	nonRevertiveInfo          api.NonRevertiveInfos
	explicitDeletes           *api.DeletePathSet
	poolFactory               pool.VirtualPoolFactory
	confirmedAppliedValues    api.ConfirmedAppliedValueResolver
	learnedMutateOnWritePaths api.LearnedMutateOnWritePathSet
}

func NewTreeContext(sc schemaClient.SchemaClientBound, poolFactory pool.VirtualPoolFactory, opts ...TreeContextOption) *TreeContext {
	tc := &TreeContext{
		schemaClient:     sc,
		nonRevertiveInfo: api.NewNonRevertiveInfos(),
		explicitDeletes:  api.NewDeletePaths(),
		poolFactory:      poolFactory,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(tc)
		}
	}
	return tc
}

// deepCopy root is required to be set manually
func (t *TreeContext) DeepCopy() api.TreeContext {
	tc := &TreeContext{
		schemaClient:              t.schemaClient,
		poolFactory:               t.poolFactory,
		confirmedAppliedValues:    t.confirmedAppliedValues,
		learnedMutateOnWritePaths: t.learnedMutateOnWritePaths,
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

func (t *TreeContext) ConfirmedAppliedValue(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue {
	if t == nil || t.confirmedAppliedValues == nil {
		return nil
	}
	return t.confirmedAppliedValues.Lookup(path, owner, intendedValue)
}

func (t *TreeContext) IsLearnedMutateOnWritePath(path *sdcpb.Path) bool {
	if t == nil || t.learnedMutateOnWritePaths == nil {
		return false
	}
	return t.learnedMutateOnWritePaths.IsLearnedMutateOnWritePath(path)
}
