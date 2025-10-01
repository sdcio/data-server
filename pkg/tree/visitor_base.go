package tree

// BaseVisitor abstract base visitor implementation that all the concrete visitory are ment to embed.
type BaseVisitor struct{}

func (b *BaseVisitor) Up() {
	// noop
}

func (b *BaseVisitor) DescendMethod() DescendMethod {
	return DescendMethodAll
}

type BaseParallelVisitor[T any] struct {
}

func (v *BaseParallelVisitor[T]) Attach(parent *T, child *T) {}
func (v *BaseParallelVisitor[T]) Detach(parent *T, child *T) {}
func (v *BaseParallelVisitor[T]) Init() error                { return nil }
func (v *BaseParallelVisitor[T]) End() error                 { return nil }
func (v *BaseParallelVisitor[T]) NewNode(e Entry) *T         { return nil }
