package datastore

import (
	"context"
	"testing"

	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/tree/importer"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// TestPopulateSensitivePathIndex_NarrowIntentReader verifies
// populateSensitivePathIndex loads sensitive_paths from every intent that
// declares them, skipping intents with none. It is built against a
// MockBoundIntentReader — not the full MockCacheClientBound — since
// populateSensitivePathIndex only ever reads real Intents via forEachIntent.
func TestPopulateSensitivePathIndex_NarrowIntentReader(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sensitivePaths := []*sdcpb.Path{
		{Elem: []*sdcpb.PathElem{{Name: "bgp"}, {Name: "neighbors"}, {Name: "auth-password"}}, IsRootBased: true},
	}
	withPaths := &tree_persist.Intent{IntentName: "marker-intent", Priority: 10, SensitivePaths: sensitivePaths}
	withoutPaths := &tree_persist.Intent{IntentName: "plain-intent", Priority: 20}

	reader := mockcacheclient.NewMockBoundIntentReader(ctrl)
	reader.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error) {
			intentChan <- treeproto.NewProtoTreeImporter(withPaths)
			intentChan <- treeproto.NewProtoTreeImporter(withoutPaths)
			close(intentChan)
			close(errChan)
		})

	s := treetypes.NewSensitivePathIndex()
	if err := populateSensitivePathIndex(ctx, s, reader); err != nil {
		t.Fatalf("populateSensitivePathIndex() error = %v", err)
	}

	if !s.Contains(sensitivePaths[0]) {
		t.Errorf("expected %s to be marked sensitive after populateSensitivePathIndex", sensitivePaths[0].ToXPath(true))
	}
}
