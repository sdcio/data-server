package api

import (
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ConfirmedAppliedValueResolver resolves the confirmed applied value observed
// on the target for a mutate-on-write leaf's canonical path, winning owner, and
// current intended value. Lookup returns nil when no baseline matches.
type ConfirmedAppliedValueResolver interface {
	Lookup(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue
}

type TreeContext interface {
	PoolFactory() pool.VirtualPoolFactory
	SchemaClient() schemaClient.SchemaClientBound
	DeepCopy() TreeContext
	ExplicitDeletes() *DeletePathSet
	NonRevertiveInfo() NonRevertiveInfos
	// ConfirmedAppliedValue returns the confirmed applied value matching path, owner, and intendedValue, or nil when none matches.
	ConfirmedAppliedValue(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue
}
