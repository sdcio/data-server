package api

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TreeContext interface {
	DeepCopy() TreeContext
	GetPoolFactory() pool.VirtualPoolFactory
	AddExplicitDeletes(intentName string, priority int32, pathset *sdcpb.PathSet)
	RemoveExplicitDeletes(intentName string) *sdcpb.PathSet
	AddNonRevertiveInfo(intent string, nonRevertive bool)
	IsNonRevertiveIntent(intent string) bool
	SetRoot(e Entry) error
	GetActualOwner() string
	SetActualOwner(owner string)
	GetSchemaClient() schemaClient.SchemaClientBound
	GetNonRevertiveInfo(intent string) bool
	GetExplicitDeletes() *DeletePathSet
}
