package datastore

import (
	"context"
	"testing"

	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/tree/importer"
	treeproto "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/sensitive"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// TestPopulateSensitivePathIndex_NarrowIntentReader loads sensitive paths from
// intents that declare them and skips intents with none. The reader is a
// BoundIntentReader; the index only needs intent reads.
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
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- importer.IntentAdapter, errChan chan<- error) {
			intentChan <- treeproto.NewProtoTreeImporter(withPaths)
			intentChan <- treeproto.NewProtoTreeImporter(withoutPaths)
			close(intentChan)
			close(errChan)
		})

	s := sensitive.NewPathIndex()
	if err := populateSensitivePathIndex(ctx, s, reader); err != nil {
		t.Fatalf("populateSensitivePathIndex() error = %v", err)
	}

	if !s.Contains(sensitivePaths[0]) {
		t.Errorf("expected %s to be marked sensitive after populateSensitivePathIndex", sensitivePaths[0].ToXPath(true))
	}
}
