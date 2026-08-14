package validation

import (
	"context"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestNavigateLeafRef_unionMatchedBranch(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	req := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name: ygot.String("ethernet-1/1"),
			},
		},
		Unionleafreftest: ygot.String("ethernet-1/1"),
	}
	if _, err := testhelper.LoadYgotStructIntoTreeRoot(ctx, req, root.Entry, "owner1", 5, false, types.NewUpdateInsertFlags()); err != nil {
		t.Fatal(err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	path := &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("unionleafreftest", nil)}}
	entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, path)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := navigateLeafRef(ctx, entry)
	if err != nil {
		t.Fatalf("navigateLeafRef: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("navigateLeafRef: want 1 target, got %d", len(entries))
	}
	last := entries[0].SdcpbPath().GetElem()
	if len(last) == 0 || last[len(last)-1].GetName() != "name" {
		t.Fatalf("navigateLeafRef: want leaf name, last elem = %v", last)
	}

	adapter := newYangParserEntryAdapter(ctx, entry)
	next, err := adapter.FollowLeafRef()
	if err != nil {
		t.Fatalf("FollowLeafRef: %v", err)
	}
	nae, ok := next.(*yangParserEntryAdapter)
	if !ok {
		t.Fatalf("FollowLeafRef: got %T", next)
	}
	el := nae.e.SdcpbPath().GetElem()
	if len(el) == 0 || el[len(el)-1].GetName() != "name" {
		t.Fatalf("FollowLeafRef: want leaf name, last elem = %v", el)
	}
}
