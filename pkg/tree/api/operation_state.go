package api

// OperationState holds mutable state accumulated during a tree operation
// (e.g. an import pass or transaction). It is created once per tree root and
// deep-copied when the tree is branched.
type OperationState interface {
	ExplicitDeletes() *DeletePathSet
	NonRevertiveInfo() NonRevertiveInfos
	DeepCopyState() OperationState
}

// operationState is the concrete, package-private implementation.
type operationState struct {
	explicitDeletes  *DeletePathSet
	nonRevertiveInfo NonRevertiveInfos
}

// NewOperationState creates a fresh OperationState with empty collections.
func NewOperationState() OperationState {
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

func (o *operationState) DeepCopyState() OperationState {
	return &operationState{
		explicitDeletes:  o.explicitDeletes.DeepCopy(),
		nonRevertiveInfo: o.nonRevertiveInfo.DeepCopy(),
	}
}
