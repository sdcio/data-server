package tree

// BaseVisitor abstract base visitor implementation that all the concrete visitory are ment to embed.
type BaseVisitor struct{}

func (b *BaseVisitor) Up() {
	// noop
}

func (b *BaseVisitor) DescendMethod() DescendMethod {
	return DescendMethodAll
}
