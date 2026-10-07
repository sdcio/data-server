package ops_test

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// inactiveCaseTree returns a tree in which Running holds a value in case1, while another owner with a higher
// precedence selects case2, so case1 is not active.
func inactiveCaseTree(t *testing.T, ctx context.Context, tp *pool.SharedTaskPool) *tree.RootEntry {
	t.Helper()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(schemaClient.NewSchemaClientBound(schema, sc), tp))
	if err != nil {
		t.Fatal(err)
	}
	imp := func(c *sdcio_schema.SdcioModel_Choices, owner string, prio int32) {
		str, err := ygot.EmitJSON(&sdcio_schema.Device{Choices: c}, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal([]byte(str), &v); err != nil {
			t.Fatal(err)
		}
		if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, owner, prio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
			t.Fatal(err)
		}
	}
	imp(&sdcio_schema.SdcioModel_Choices{Case1: &sdcio_schema.SdcioModel_Choices_Case1{
		CaseElem: &sdcio_schema.SdcioModel_Choices_Case1_CaseElem{Elem: ygot.String("running")},
	}}, consts.RunningIntentName, consts.RunningValuesPrio)
	imp(&sdcio_schema.SdcioModel_Choices{Case2: &sdcio_schema.SdcioModel_Choices_Case2{Log: ygot.Bool(true)}}, "intent", 5)
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestDeleteBranchEntryInactiveCase proves that a Running branch in an inactive case is deleted when it is
// given by entry. DeleteBranch navigates along the active cases only, so it does not find such a branch by
// its path and leaves it in the tree.
func TestDeleteBranchEntryInactiveCase(t *testing.T) {
	ctx := context.Background()
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	root := inactiveCaseTree(t, ctx, tp)

	choices, ok := root.Entry.GetChildMap().GetEntry("choices")
	if !ok {
		t.Fatal("choices missing")
	}
	case1, ok := choices.GetChildMap().GetEntry("case1")
	if !ok {
		t.Fatal("case1 missing")
	}

	if err := ops.DeleteBranchEntry(ctx, case1, consts.RunningIntentName); err != nil {
		t.Fatal(err)
	}
	if _, ok := choices.GetChildMap().GetEntry("case1"); ok {
		t.Fatal("case1 of the inactive case must be gone from the tree")
	}
	// the other owner is not affected
	if _, ok := choices.GetChildMap().GetEntry("case2"); !ok {
		t.Fatal("case2 of the other owner must remain")
	}
}
