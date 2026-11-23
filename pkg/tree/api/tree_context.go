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

// LearnedMutateOnWritePathSet reports whether a leaf's keyless schema path has
// been dynamically learned as mutate-on-write for the target.
type LearnedMutateOnWritePathSet interface {
	IsLearnedMutateOnWritePath(path *sdcpb.Path) bool
}

type TreeContext interface {
	PoolFactory() pool.VirtualPoolFactory
	SchemaClient() schemaClient.SchemaClientBound
	DeepCopy() TreeContext
	ExplicitDeletes() *DeletePathSet
	NonRevertiveInfo() NonRevertiveInfos
	// ConfirmedAppliedValue returns the confirmed applied value matching path, owner, and intendedValue, or nil when none matches.
	ConfirmedAppliedValue(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue
	// IsLearnedMutateOnWritePath reports whether path's keyless schema path was dynamically learned as mutate-on-write.
	IsLearnedMutateOnWritePath(path *sdcpb.Path) bool
	// SetLearnedMutateOnWritePaths replaces the dynamic mutate-on-write path lookup used for this tree context.
	SetLearnedMutateOnWritePaths(learned LearnedMutateOnWritePathSet)
}

// IsMutateOnWriteEntry reports whether e is classified as a mutate-on-write
// leaf by its YANG schema or by its tree context's learned path set.
func IsMutateOnWriteEntry(e Entry) bool {
	if e == nil {
		return false
	}
	if e.GetSchema().IsMutateOnWrite() {
		return true
	}
	if tc := e.GetTreeContext(); tc != nil {
		return tc.IsLearnedMutateOnWritePath(e.SdcpbPath())
	}
	return false
}
