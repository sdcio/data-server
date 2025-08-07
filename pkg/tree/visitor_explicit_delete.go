package tree

import (
	"context"
)

type ExplicitDeleteVisitor struct {
}

var _ EntryVisitor = (*ExplicitDeleteVisitor)(nil)

func NewExplicitDeleteVisitor() *ExplicitDeleteVisitor {
	return &ExplicitDeleteVisitor{}
}

func (edv *ExplicitDeleteVisitor) Visit(ctx context.Context, e Entry) error {

	return nil
}
func (edv *ExplicitDeleteVisitor) Up() {
	// noop
}
