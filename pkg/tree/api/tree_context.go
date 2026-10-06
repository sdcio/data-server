package api

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
)

type TreeContext interface {
	PoolFactory() pool.VirtualPoolFactory
	SchemaClient() schemaClient.SchemaClientBound
	DeepCopy() TreeContext
	ExplicitDeletes() *DeletePathSet
	DeletePathCoverage() *DeletePathSet
	ResetDeletePathCoverage()
	SetDeletePathCoverage(*DeletePathSet)
	NonRevertiveInfo() NonRevertiveInfos
}
