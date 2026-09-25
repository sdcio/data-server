package types

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type RunningStore interface {
	ApplyToRunning(ctx context.Context, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error
	NewEmptyTree(ctx context.Context) (*tree.RootEntry, error)
	// MarkSynced records that the named sync mechanism has completed its
	// first successful cycle writing to Running. Idempotent: the first
	// success for a given name latches; later calls for the same (or an
	// already fully-synced) name are no-ops.
	MarkSynced(name string)
}
