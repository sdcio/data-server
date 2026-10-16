package validation_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	json_importer "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// Regression test for the sonic-spine1 SRv6 locator investigation: a
// count(leaflist[predicate]) must-statement, like SONiC's
// "count(adv_speeds[text()='all']) = 0 or count(adv_speeds) = 1", must not
// crash the validator regardless of whether the leaf-list is populated or
// entirely unset. yangParserEntryAdapter.GetValue() used to return an
// empty NodesetDatum for an unset leaf-list, which fell through Eq()'s new
// DatumSlice handling into the pre-existing list-key-equality shortcut,
// still producing a bare Bool for count() to choke on ("Fn 'count' takes
// NODESET, not BOOL as arg 0").
func TestValidate_Must_CountLeafListPredicate(t *testing.T) {
	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name:       "unset leaf-list - must not crash, must pass",
			req:        &sdcio_schema.Device{},
			wantErrors: 0,
		},
		{
			name: "single 'all' flag - passes",
			req: &sdcio_schema.Device{
				CountLeaflist: &sdcio_schema.SdcioModel_CountLeaflist{
					Flag: []string{"all"},
				},
			},
			wantErrors: 0,
		},
		{
			name: "single non-'all' flag - passes",
			req: &sdcio_schema.Device{
				CountLeaflist: &sdcio_schema.SdcioModel_CountLeaflist{
					Flag: []string{"foo"},
				},
			},
			wantErrors: 0,
		},
		{
			name: "'all' alongside another flag - fails",
			req: &sdcio_schema.Device{
				CountLeaflist: &sdcio_schema.SdcioModel_CountLeaflist{
					Flag: []string{"all", "foo"},
				},
			},
			wantErrors: 1,
		},
	}

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			jsonConfStr, err := ygot.EmitJSON(tt.req, &ygot.EmitJSONConfig{
				Format:         ygot.RFC7951,
				SkipValidation: true,
			})
			if err != nil {
				t.Fatal(err)
			}

			var jsonConfAny any
			if err := json.Unmarshal([]byte(jsonConfStr), &jsonConfAny); err != nil {
				t.Fatal(err)
			}

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			jimporter := json_importer.NewJsonTreeImporter(jsonConfAny, "owner1", 5, false)
			if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool); err != nil {
				t.Fatal(err)
			}

			if err := root.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			valConf := config.NewValidationConfig()
			valConf.SetDisableConcurrency(true)

			result, _ := validation.Validate(ctx, root.Entry, valConf, sharedPool)

			t.Logf("Validation Errors:\n%s", strings.Join(result.ErrorsStr(), "\n"))

			if len(result.ErrorsStr()) != tt.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tt.wantErrors, len(result.ErrorsStr()), result.ErrorsStr())
			}

			for _, errStr := range result.ErrorsStr() {
				if strings.Contains(errStr, "Fn 'count' takes NODESET, not BOOL") {
					t.Fatalf("must-statement crashed instead of evaluating: %s", errStr)
				}
			}
		})
	}
}
