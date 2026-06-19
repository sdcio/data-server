package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	"go.uber.org/mock/gomock"
)

// unionIngressMatrixRow is one observable validation scenario for issue 008 / PRD 11–12.
// Maintainer-facing ingress summary: docs/prd/union-member-resolution-validation/UNION-INGRESS.md
//
// Structured JSON import (ygot → tree) is the oracle; expansion paths must match error counts
// when the submitted encoding determinately selects a union member (inferable cases).
//
// Ambiguous shapes (e.g. two string union branches) are not present on the integration device
// model; see pkg/tree/importer/union_member_test.go and XML/proto importer tests for RFC 7950
// §9.12 first-member union matching there.
type unionIngressMatrixRow struct {
	name       string
	category   string // pattern | range | length | leafref — PRD-named constraint families
	req        *sdcio_schema.Device
	wantErrors int
}

func unionIngressMatrixCases() []unionIngressMatrixRow {
	return []unionIngressMatrixRow{
		// Pattern (union string branch carries regex; uint32 branch does not)
		{
			name:     "inferable_pattern_string_branch_ok",
			category: "pattern",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantErrors: 0,
		},
		{
			name:     "inferable_pattern_string_branch_violation",
			category: "pattern",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hello AB"},
			},
			wantErrors: 1,
		},
		{
			name:     "inferable_pattern_uint32_branch_no_pattern",
			category: "pattern",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_Uint32{Uint32: 42},
			},
			wantErrors: 0,
		},
		// Range
		{
			name:     "inferable_range_uint32_in_range",
			category: "range",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 100},
			},
			wantErrors: 0,
		},
		{
			name:     "inferable_range_uint32_out_of_range",
			category: "range",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 5},
			},
			wantErrors: 1,
		},
		{
			name:     "inferable_range_string_branch_no_range",
			category: "range",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_String{String: "hello"},
			},
			wantErrors: 0,
		},
		// Length
		{
			name:     "inferable_length_string_branch_ok",
			category: "length",
			req: &sdcio_schema.Device{
				Unionlengthtest: &sdcio_schema.SdcioModel_Unionlengthtest_Union_String{String: "12345678"},
			},
			wantErrors: 0,
		},
		{
			name:     "inferable_length_string_branch_too_short",
			category: "length",
			req: &sdcio_schema.Device{
				Unionlengthtest: &sdcio_schema.SdcioModel_Unionlengthtest_Union_String{String: "12345"},
			},
			wantErrors: 1,
		},
		{
			name:     "inferable_length_uint32_branch_no_length",
			category: "length",
			req: &sdcio_schema.Device{
				Unionlengthtest: &sdcio_schema.SdcioModel_Unionlengthtest_Union_Uint32{Uint32: 42},
			},
			wantErrors: 0,
		},
		// Leafref (union member must be the leafref branch for required-instance resolution)
		{
			name:     "inferable_leafref_resolves_when_target_exists",
			category: "leafref",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {Name: ygot.String("ethernet-1/1")},
				},
				Unionleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors: 0,
		},
		{
			name:     "inferable_leafref_errors_when_target_missing",
			category: "leafref",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/2": {Name: ygot.String("ethernet-1/2")},
				},
				Unionleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors: 1,
		},
	}
}

// TestValidate_Union_IngressMatrix_IntentExpandVsStructuredJSON runs issue 008 matrix under
// northbound-style Intent expansion (PRD 001 / ExpandAndConvertIntent vs structured JSON).
func TestValidate_Union_IngressMatrix_IntentExpandVsStructuredJSON(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	valConf := validationConfig
	valConfLeafrefOnly := valConf.DeepCopy()
	valConfLeafrefOnly.DisabledValidators.DisableAll()
	valConfLeafrefOnly.DisabledValidators.Leafref = false

	for _, row := range unionIngressMatrixCases() {
		t.Run(row.category+"/"+row.name, func(t *testing.T) {
			vc := valConf
			if row.category == "leafref" {
				vc = valConfLeafrefOnly
			}

			rootJSON, poolJSON := importDeviceJSON(t, ctx, scb, row.req)
			resJSON, _ := validation.Validate(ctx, rootJSON.Entry, vc, poolJSON)
			if got := len(resJSON.ErrorsStr()); got != row.wantErrors {
				t.Fatalf("structured JSON: want %d errors, got %d: %v",
					row.wantErrors, got, resJSON.ErrorsStr())
			}

			rootExpand, poolExpand := importDeviceIntentJSON(t, ctx, scb, row.req)
			resExpand, _ := validation.Validate(ctx, rootExpand.Entry, vc, poolExpand)
			t.Logf("intent expand errors:\n%s", strings.Join(resExpand.ErrorsStr(), "\n"))
			if got := len(resExpand.ErrorsStr()); got != row.wantErrors {
				t.Errorf("intent expansion: want %d errors (match structured), got %d: %v",
					row.wantErrors, got, resExpand.ErrorsStr())
			}
		})
	}
}

// TestValidate_Union_IngressMatrix_RunningExpandVsStructuredJSON runs the same matrix under
// Running owner/priority (PRD 002 / gNMI Sync expansion parity).
func TestValidate_Union_IngressMatrix_RunningExpandVsStructuredJSON(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	valConf := validationConfig
	valConfLeafrefOnly := valConf.DeepCopy()
	valConfLeafrefOnly.DisabledValidators.DisableAll()
	valConfLeafrefOnly.DisabledValidators.Leafref = false

	for _, row := range unionIngressMatrixCases() {
		t.Run(row.category+"/"+row.name, func(t *testing.T) {
			vc := valConf
			if row.category == "leafref" {
				vc = valConfLeafrefOnly
			}

			rootJSON, poolJSON := importDeviceJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, row.req)
			resJSON, _ := validation.Validate(ctx, rootJSON.Entry, vc, poolJSON)
			if got := len(resJSON.ErrorsStr()); got != row.wantErrors {
				t.Fatalf("structured JSON (running): want %d errors, got %d: %v",
					row.wantErrors, got, resJSON.ErrorsStr())
			}

			rootExpand, poolExpand := importDeviceExpandJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, row.req)
			resExpand, _ := validation.Validate(ctx, rootExpand.Entry, vc, poolExpand)
			t.Logf("running expand errors:\n%s", strings.Join(resExpand.ErrorsStr(), "\n"))
			if got := len(resExpand.ErrorsStr()); got != row.wantErrors {
				t.Errorf("running expansion: want %d errors (match structured), got %d: %v",
					row.wantErrors, got, resExpand.ErrorsStr())
			}
		})
	}
}

// TestValidate_Union_IngressMatrix_OptionalLeafref_ExpandVsStructuredJSON asserts best-effort
// behaviour (warning, not error) for optional-instance union leafref matches between structured
// import and expansion paths — PRD 12 / B semantics for inferable vs fallback.
func TestValidate_Union_IngressMatrix_OptionalLeafref_ExpandVsStructuredJSON(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	valConf := config.NewValidationConfig()
	valConf.DisabledValidators.DisableAll()
	valConf.DisabledValidators.Leafref = false
	valConf.SetDisableConcurrency(true)

	cases := []struct {
		name           string
		req            *sdcio_schema.Device
		wantErrors     int
		wantWarnings   int
		useRunningPrio bool
	}{
		{
			name: "ambiguous_optional_leafref_missing_target_warning_intent",
			req: &sdcio_schema.Device{
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:     0,
			wantWarnings:   1,
			useRunningPrio: false,
		},
		{
			name: "ambiguous_optional_leafref_resolves_no_warning_intent",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {Name: ygot.String("ethernet-1/1")},
				},
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:     0,
			wantWarnings:   0,
			useRunningPrio: false,
		},
		{
			name: "ambiguous_optional_leafref_missing_target_warning_running",
			req: &sdcio_schema.Device{
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:     0,
			wantWarnings:   1,
			useRunningPrio: true,
		},
		{
			name: "ambiguous_optional_leafref_resolves_no_warning_running",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {Name: ygot.String("ethernet-1/1")},
				},
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:     0,
			wantWarnings:   0,
			useRunningPrio: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rootJSON, rootExpand *tree.RootEntry
			var poolJSON, poolExpand *pool.SharedTaskPool

			if tc.useRunningPrio {
				rootJSON, poolJSON = importDeviceJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, tc.req)
				rootExpand, poolExpand = importDeviceExpandJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, tc.req)
			} else {
				rootJSON, poolJSON = importDeviceJSON(t, ctx, scb, tc.req)
				rootExpand, poolExpand = importDeviceIntentJSON(t, ctx, scb, tc.req)
			}

			resJSON, _ := validation.Validate(ctx, rootJSON.Entry, valConf, poolJSON)
			if len(resJSON.ErrorsStr()) != tc.wantErrors || len(resJSON.WarningsStr()) != tc.wantWarnings {
				t.Fatalf("structured JSON: want %d errors %d warnings, got %d errors %d warnings\nerrors=%v\nwarnings=%v",
					tc.wantErrors, tc.wantWarnings, len(resJSON.ErrorsStr()), len(resJSON.WarningsStr()),
					resJSON.ErrorsStr(), resJSON.WarningsStr())
			}

			resExpand, _ := validation.Validate(ctx, rootExpand.Entry, valConf, poolExpand)
			t.Logf("expand errors=%v warnings=%v", resExpand.ErrorsStr(), resExpand.WarningsStr())
			if len(resExpand.ErrorsStr()) != tc.wantErrors || len(resExpand.WarningsStr()) != tc.wantWarnings {
				t.Errorf("expansion path: want %d errors %d warnings (match structured), got %d errors %d warnings\nerrors=%v\nwarnings=%v",
					tc.wantErrors, tc.wantWarnings, len(resExpand.ErrorsStr()), len(resExpand.WarningsStr()),
					resExpand.ErrorsStr(), resExpand.WarningsStr())
			}
		})
	}
}
