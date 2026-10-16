package api

import (
	"context"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/schema"
)

type TreeContext interface {
	PoolFactory() pool.VirtualPoolFactory
	SchemaClient() schemaClient.SchemaClientBound
	RootAmbiguityRegistry(ctx context.Context) (schema.RootAmbiguityRegistry, error)
	DeepCopy() TreeContext
	ExplicitDeletes() *DeletePathSet
	NonRevertiveInfo() NonRevertiveInfos
}
