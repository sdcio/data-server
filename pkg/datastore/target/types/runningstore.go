package types

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/sdc-protos/tree_persist"
)

type RunningStore interface {
	ApplyToRunning(ctx context.Context, i *tree_persist.Intent) error
	NewEmptyTree(ctx context.Context) (*tree.RootEntry, error)
}
