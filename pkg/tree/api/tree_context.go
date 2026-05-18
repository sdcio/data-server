package api

type TreeContext interface {
	GetTreeConfig() TreeConfig
	GetOperationState() OperationState
	DeepCopy() TreeContext
}
