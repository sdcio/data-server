package validation_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
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

// validatePatterntest validates a tree holding the given value for the
// patterntest leaf (pattern 'hallo [0-9a-fA-F]*') and returns the errors.
func validatePatterntest(t *testing.T, value string) []string {
	t.Helper()
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, sharedPool)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	jsonStr, err := ygot.EmitJSON(&sdcio_schema.Device{Patterntest: ygot.String(value)}, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	var jsonConfig any
	if err := json.Unmarshal([]byte(jsonStr), &jsonConfig); err != nil {
		t.Fatal(err)
	}

	jimporter := json_importer.NewJsonTreeImporter(jsonConfig, "owner1", 5, false)
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool); err != nil {
		t.Fatal(err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	res, _ := validation.Validate(ctx, root.Entry, validationConfig.DeepCopy(), sharedPool)
	return res.ErrorsStr()
}

func TestValidate_Pattern(t *testing.T) {
	// Run each case twice: the second run is served from the pattern cache and
	// must produce the identical outcome.
	for run := 1; run <= 2; run++ {
		if errs := validatePatterntest(t, "hallo 00"); len(errs) != 0 {
			t.Errorf("run %d: valid value produced errors: %v", run, errs)
		}

		errs := validatePatterntest(t, "hallo xyz")
		if len(errs) != 1 {
			t.Fatalf("run %d: expected 1 error, got %d: %v", run, len(errs), errs)
		}
		for _, want := range []string{
			"value hallo xyz of /patterntest does not match regex",
			`schema: hallo [0-9a-fA-F]*`,
			`goPattern: ^(?:hallo [0-9a-fA-F]*)$`,
			"inverted: false",
		} {
			if !strings.Contains(errs[0], want) {
				t.Errorf("run %d: error %q does not contain %q", run, errs[0], want)
			}
		}
	}
}
