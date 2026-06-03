package api

// TreeOperationState holds mutable state accumulated during a tree operation
// (e.g. an import pass or transaction). It is created once per tree root and
// deep-copied when the tree is branched.
type TreeOperationState interface {
	ExplicitDeletes() *DeletePathSet
	NonRevertiveInfo() NonRevertiveInfos
	DeepCopy() TreeOperationState
}

// operationState is the concrete, package-private implementation.
type operationState struct {
	explicitDeletes  *DeletePathSet
	nonRevertiveInfo NonRevertiveInfos
}

// NewTreeOperationState creates a fresh TreeOperationState with empty collections.
func NewTreeOperationState() TreeOperationState {
	return &operationState{
		explicitDeletes:  NewDeletePaths(),
		nonRevertiveInfo: NewNonRevertiveInfos(),
	}
}

func (o *operationState) ExplicitDeletes() *DeletePathSet {
	return o.explicitDeletes
}

func (o *operationState) NonRevertiveInfo() NonRevertiveInfos {
	return o.nonRevertiveInfo
}

func (o *operationState) DeepCopy() TreeOperationState {
	return &operationState{
		explicitDeletes:  o.explicitDeletes.DeepCopy(),
		nonRevertiveInfo: o.nonRevertiveInfo.DeepCopy(),
	}
}
