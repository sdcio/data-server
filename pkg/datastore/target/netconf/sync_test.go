package netconf

import (
	"context"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type timestampRunningStore struct {
	syncStartedAt int64
}

func (s *timestampRunningStore) ApplyToRunning(_ context.Context, syncStartedAt int64, _ []*sdcpb.Path, _ importer.ImportConfigAdapter) error {
	s.syncStartedAt = syncStartedAt
	return nil
}

func (*timestampRunningStore) NewEmptyTree(context.Context) (*tree.RootEntry, error) {
	return nil, nil
}

type staticXMLImporterTarget struct{}

func (staticXMLImporterTarget) GetImportAdapter(context.Context, *sdcpb.GetDataRequest) (importer.ImportConfigAdapter, error) {
	return nil, nil
}

func TestNetconfSync_PassesSnapshotStartToRunning(t *testing.T) {
	store := &timestampRunningStore{}
	syncer := &NetconfSyncImpl{
		ctx:          context.Background(),
		target:       staticXMLImporterTarget{},
		runningStore: store,
	}

	before := time.Now().UnixNano()
	if err := syncer.internalSync(&sdcpb.GetDataRequest{}); err != nil {
		t.Fatalf("internalSync: %v", err)
	}
	after := time.Now().UnixNano()

	if store.syncStartedAt < before || store.syncStartedAt > after {
		t.Fatalf("syncStartedAt=%d, want timestamp in [%d,%d]", store.syncStartedAt, before, after)
	}
}
