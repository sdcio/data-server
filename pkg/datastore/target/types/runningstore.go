package types

import (
	"context"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type RunningStore interface {
	// ApplyToRunning commits a sync observation whose Unix-nanosecond start
	// timestamp was captured before the producer began reading the target.
	ApplyToRunning(ctx context.Context, syncStartedAt int64, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error
	NewEmptyTree(ctx context.Context) (*tree.RootEntry, error)
}

// InitialSyncStore is optionally implemented by a RunningStore that atomically
// records configured syncPaths coverage alongside an initial merge-only sync
// (such as the first StreamSync SyncResponse), even when importer is nil.
type InitialSyncStore interface {
	ApplyInitialSync(ctx context.Context, syncStartedAt int64, syncPaths []*sdcpb.Path, deletes []*sdcpb.Path, importer importer.ImportConfigAdapter) error
}
