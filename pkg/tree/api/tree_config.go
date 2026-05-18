package api

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
)

// TreeConfig holds immutable tree-setup values. It is created once per tree
// root and shared (by identity) across all DeepCopy calls.
type TreeConfig interface {
	SchemaClient() schemaClient.SchemaClientBound
	PoolFactory() pool.VirtualPoolFactory
}
