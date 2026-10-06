package tree

import (
	"context"
	"runtime"
	"slices"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestGetChildHonoursChoiceSkipList(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, err := NewTreeRoot(ctx, NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))))
	if err != nil {
		t.Fatal(err)
	}
	converter := utils.NewConverter(scb)
	case1 := &sdcio_schema.Device{
		Choices: &sdcio_schema.SdcioModel_Choices{
			Case1: &sdcio_schema.SdcioModel_Choices_Case1{
				CaseElem: &sdcio_schema.SdcioModel_Choices_Case1_CaseElem{
					Elem: ygot.String("case1-val"),
				},
			},
		},
	}
	case2 := &sdcio_schema.Device{
		Choices: &sdcio_schema.SdcioModel_Choices{
			Case2: &sdcio_schema.SdcioModel_Choices_Case2{
				Log: ygot.Bool(true),
			},
		},
	}
	case1Upds, err := testhelper.ExpandUpdateFromConfig(ctx, case1, converter)
	if err != nil {
		t.Fatal(err)
	}
	case2Upds, err := testhelper.ExpandUpdateFromConfig(ctx, case2, converter)
	if err != nil {
		t.Fatal(err)
	}
	if err = testhelper.AddToRoot(ctx, root.Entry, case1Upds, testhelper.FlagsNew, "owner1", 5); err != nil {
		t.Fatal(err)
	}
	if err = testhelper.AddToRoot(ctx, root.Entry, case2Upds, testhelper.FlagsNew, "owner2", 50); err != nil {
		t.Fatal(err)
	}
	if err = root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	choices, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("choices", nil)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := choices.GetChild("case1", types.DescendMethodAll); !ok {
		t.Fatal("case1 missing from all children")
	}
	if _, ok := choices.GetChild("case2", types.DescendMethodAll); !ok {
		t.Fatal("case2 missing from all children")
	}

	active := choices.GetChilds(types.DescendMethodActiveChilds)
	all := choices.GetChilds(types.DescendMethodAll)
	if _, ok := all["case1"]; !ok {
		t.Fatal("GetChilds(All) missing case1")
	}
	if _, ok := all["case2"]; !ok {
		t.Fatal("GetChilds(All) missing case2")
	}

	_, case1Active := choices.GetChild("case1", types.DescendMethodActiveChilds)
	_, case2Active := choices.GetChild("case2", types.DescendMethodActiveChilds)
	_, case1InMap := active["case1"]
	_, case2InMap := active["case2"]
	if case1Active != case1InMap {
		t.Fatalf("GetChild(case1, ActiveChilds)=%v, GetChilds map has case1=%v", case1Active, case1InMap)
	}
	if case2Active != case2InMap {
		t.Fatalf("GetChild(case2, ActiveChilds)=%v, GetChilds map has case2=%v", case2Active, case2InMap)
	}
	if !case1Active {
		t.Fatal("higher-priority case1 should be active")
	}
	if case2Active {
		t.Fatal("lower-priority case2 should be skipped by the choice skip list")
	}

	activeNames := childNames(choices.SnapshotChilds(types.DescendMethodActiveChilds))
	allNames := childNames(choices.SnapshotChilds(types.DescendMethodAll))
	if !slices.Contains(allNames, "case1") || !slices.Contains(allNames, "case2") {
		t.Fatalf("SnapshotChilds(All) = %v, want case1 and case2", allNames)
	}
	if !slices.Contains(activeNames, "case1") || slices.Contains(activeNames, "case2") {
		t.Fatalf("SnapshotChilds(ActiveChilds) = %v, want case1 only", activeNames)
	}
}

func TestLeafGetChildsDoesNotAllocate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, err := NewTreeRoot(ctx, NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))))
	if err != nil {
		t.Fatal(err)
	}
	converter := utils.NewConverter(scb)
	upds, err := testhelper.ExpandUpdateFromConfig(ctx, testhelper.Config1(), converter)
	if err != nil {
		t.Fatal(err)
	}
	if err = testhelper.AddToRoot(ctx, root.Entry, upds, testhelper.FlagsNew, "owner1", 5); err != nil {
		t.Fatal(err)
	}

	leaf, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
			sdcpb.NewPathElem("description", nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		got := leaf.GetChilds(types.DescendMethodAll)
		if len(got) != 0 {
			t.Fatalf("leaf GetChilds len = %d, want 0", len(got))
		}
	})
	if allocs != 0 {
		t.Fatalf("leaf GetChilds allocated %.2f times per run, want 0", allocs)
	}

	allocs = testing.AllocsPerRun(1000, func() {
		got := leaf.SnapshotChilds(types.DescendMethodAll)
		if len(got) != 0 {
			t.Fatalf("leaf SnapshotChilds len = %d, want 0", len(got))
		}
	})
	if allocs != 0 {
		t.Fatalf("leaf SnapshotChilds allocated %.2f times per run, want 0", allocs)
	}
}
