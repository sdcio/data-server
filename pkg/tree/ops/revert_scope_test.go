package ops_test

import (
	"context"
	"encoding/json"
	"runtime"
	"sort"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestRevertScope(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(scb, tp))
	if err != nil {
		t.Fatal(err)
	}

	conf := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {Name: ygot.String("ethernet-1/1"), Description: ygot.String("a")},
		},
		Choices: &sdcio_schema.SdcioModel_Choices{
			Case1: &sdcio_schema.SdcioModel_Choices_Case1{CaseElem: &sdcio_schema.SdcioModel_Choices_Case1_CaseElem{Elem: ygot.String("x")}},
		},
	}
	str, err := ygot.EmitJSON(conf, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		t.Fatal(err)
	}
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
		t.Fatal(err)
	}
	// navigation only follows the active case of a choice, which is determined by this
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "leaf of a list entry is evaluated within the list entry", path: "/interface[name=ethernet-1/1]/description", want: "interface[name=ethernet-1/1]"},
		{name: "leaf of a case is lifted to the owner of the choice", path: "/choices/case1/log", want: "choices"},
		{name: "leaf below a nested container of a case is lifted to the owner of the choice", path: "/choices/case1/case-elem/elem", want: "choices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := sdcpb.ParsePath(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			e, err := ops.NavigateSdcpbPath(ctx, root.Entry, p)
			if err != nil {
				t.Fatal(err)
			}
			if got := ops.RevertScope(e).ToXPath(false); got != tt.want {
				t.Fatalf("RevertScope(%s) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestCollectRunningSyncRevertScopes checks that the collector sees entries marked deleted, which are
// removed by the remove deleted processor afterwards, and that it yields scopes, not entry paths.
func TestCollectRunningSyncRevertScopes(t *testing.T) {
	ctx := context.Background()
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(scb, tp))
	if err != nil {
		t.Fatal(err)
	}

	emit := func(desc string) any {
		conf := &sdcio_schema.Device{
			Interface: map[string]*sdcio_schema.SdcioModel_Interface{
				"ethernet-1/1": {Name: ygot.String("ethernet-1/1"), Description: ygot.String(desc)},
			},
		}
		str, err := ygot.EmitJSON(conf, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal([]byte(str), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	imp := func(v any) {
		if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), treetypes.NewUpdateInsertFlags(), tp); err != nil {
			t.Fatal(err)
		}
	}
	scopes := func() []string {
		var got []string
		for p := range ops.CollectRunningSyncRevertScopes(root.Entry).Items() {
			got = append(got, p.ToXPath(false))
		}
		sort.Strings(got)
		return got
	}

	imp(emit("a"))
	// new entries are inserted unflagged, so there is nothing touched to collect
	if got := scopes(); len(got) != 0 {
		t.Fatalf("expected no scopes for new entries, got %v", got)
	}

	imp(emit("b"))
	got := scopes()
	if len(got) != 1 || got[0] != "interface[name=ethernet-1/1]" {
		t.Fatalf("updated leaf: got %v, want the list entry", got)
	}
	if err := processors.NewResetFlagsProcessor(&processors.ResetFlagsProcessorParams{DeleteFlag: true, NewFlag: true, UpdateFlag: true}).Run(root.Entry, tp); err != nil {
		t.Fatal(err)
	}

	// mark the whole tree deleted, the entries are still there for the collector
	if err := processors.NewOwnerDeleteMarker(&processors.OwnerDeleteMarkerProcessorParams{Owner: consts.RunningIntentName}).Run(root.Entry, tp); err != nil {
		t.Fatal(err)
	}
	got = scopes()
	if len(got) != 1 || got[0] != "interface[name=ethernet-1/1]" {
		t.Fatalf("deleted leaf: got %v, want the list entry", got)
	}
}
