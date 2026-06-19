package api

type TreeContext interface {
	TreeConfig() TreeConfig
	OperationState() TreeOperationState
	DeepCopy() TreeContext
}
