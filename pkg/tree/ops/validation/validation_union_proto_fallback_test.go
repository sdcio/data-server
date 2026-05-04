package validation_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	proto_importer "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// TestValidate_Union_ProtoFallback verifies that proto-imported union leaves —
// where matchedUnionType is nil because no original lexical context is available —
// fall back gracefully to the outer schema type in all validators.
// Expected behaviour:
//   - No panics in any validator.
//   - Pattern/length/range/leafref validators silently skip because the outer
//     union type carries none of those constraints.
//   - This matches pre-change proto import behaviour (regression guard).
func TestValidate_Union_ProtoFallback(t *testing.T) {
	tests := []struct {
		name           string
		req            *sdcio_schema.Device
		wantAfterJSON  int
		wantAfterProto int
	}{
		{
			// Valid value: matched string branch pattern ("hallo.*") satisfied.
			// After proto round-trip the outer union type has no pattern constraint,
			// so the result is still 0 errors — consistent behaviour.
			name: "valid union pattern value round-trips without error",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantAfterJSON:  0,
			wantAfterProto: 0,
		},
		{
			// Pattern-violating value: matched branch has pattern "hallo.*" which
			// "hello AB" does not satisfy → 1 error after JSON import.
			// After proto round-trip matchedType is nil → fallback to outer union
			// type (no pattern) → 0 errors. Validators must not panic.
			name: "pattern-violating union value has no error after proto round-trip (fallback)",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hello AB"},
			},
			wantAfterJSON:  1,
			wantAfterProto: 0,
		},
		{
			// Length-violating value: matched string branch has length constraint
			// min 8 chars; "123" is too short → 1 error after JSON import.
			// After proto round-trip matchedType is nil → fallback → 0 errors.
			name: "length-violating union value has no error after proto round-trip (fallback)",
			req: &sdcio_schema.Device{
				Unionlengthtest: &sdcio_schema.SdcioModel_Unionlengthtest_Union_String{String: "123"},
			},
			wantAfterJSON:  1,
			wantAfterProto: 0,
		},
		{
			// Range-violating value: matched uint32 branch has range 10..300;
			// 500 is out of range → 1 error after JSON import.
			// After proto round-trip matchedType is nil → fallback → 0 errors.
			name: "range-violating union value has no error after proto round-trip (fallback)",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 500},
			},
			wantAfterJSON:  1,
			wantAfterProto: 0,
		},
	}

	const (
		owner    = "owner1"
		priority = int32(5)
	)

	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ── Step 1: JSON import ──────────────────────────────────────────
			root, sharedPool := importDeviceJSON(t, ctx, scb, tt.req)

			// ── Step 2: validate after JSON import ──────────────────────────
			resultJSON, _ := validation.Validate(ctx, root.Entry, validationConfig, sharedPool)
			t.Logf("Validation errors after JSON import:\n%s", strings.Join(resultJSON.ErrorsStr(), "\n"))
			if got := len(resultJSON.ErrorsStr()); got != tt.wantAfterJSON {
				t.Errorf("after JSON import: expected %d error(s), got %d: %v",
					tt.wantAfterJSON, got, resultJSON.ErrorsStr())
			}

			// ── Step 3: export to proto, reload ────────────────────────────
			persisted, err := ops.TreeExport(root.Entry, owner, priority, false)
			if err != nil {
				t.Fatal(err)
			}

			tc2 := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root2, err := tree.NewTreeRoot(ctx, tc2)
			if err != nil {
				t.Fatal(err)
			}

			sharedPool2 := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			if _, err = root2.ImportConfig(ctx, &sdcpb.Path{},
				proto_importer.NewProtoTreeImporter(persisted),
				types.NewUpdateInsertFlags(), sharedPool2); err != nil {
				t.Fatal(err)
			}
			if err = root2.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			// ── Step 4: validate after proto round-trip ─────────────────────
			resultProto, _ := validation.Validate(ctx, root2.Entry, validationConfig, sharedPool2)
			t.Logf("Validation errors after proto round-trip:\n%s", strings.Join(resultProto.ErrorsStr(), "\n"))
			if got := len(resultProto.ErrorsStr()); got != tt.wantAfterProto {
				t.Errorf("after proto round-trip: expected %d error(s), got %d: %v",
					tt.wantAfterProto, got, resultProto.ErrorsStr())
			}
		})
	}
}
