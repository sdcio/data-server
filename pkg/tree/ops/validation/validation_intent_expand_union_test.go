package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	"go.uber.org/mock/gomock"
)

// TestValidate_UnionPattern_IntentExpansionMatchesStructuredJSON verifies PRD 001:
// for an inferable union string branch, expanded intent updates carry matched type so
// branch pattern validation matches structured JSON import (same error count).
func TestValidate_UnionPattern_IntentExpansionMatchesStructuredJSON(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "union string branch pattern matches",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantErrors: 0,
		},
		{
			name: "union string branch pattern violation",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hello AB"},
			},
			wantErrors: 1,
		},
		{
			name: "union uint32 branch has no pattern",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_Uint32{Uint32: 42},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootJSON, poolJSON := importDeviceJSON(t, ctx, scb, tt.req)
			resultJSON, _ := validation.Validate(ctx, rootJSON.Entry, validationConfig, poolJSON)
			if len(resultJSON.ErrorsStr()) != tt.wantErrors {
				t.Fatalf("structured JSON import: want %d errors, got %d: %v",
					tt.wantErrors, len(resultJSON.ErrorsStr()), resultJSON.ErrorsStr())
			}

			rootExpand, poolExpand := importDeviceIntentJSON(t, ctx, scb, tt.req)
			resultExpand, _ := validation.Validate(ctx, rootExpand.Entry, validationConfig, poolExpand)
			t.Logf("intent expand validation errors:\n%s", strings.Join(resultExpand.ErrorsStr(), "\n"))

			if len(resultExpand.ErrorsStr()) != tt.wantErrors {
				t.Errorf("intent expansion path: want %d errors (same as JSON import), got %d: %v",
					tt.wantErrors, len(resultExpand.ErrorsStr()), resultExpand.ErrorsStr())
			}
		})
	}
}

// TestValidate_UnionPattern_RunningExpandMatchesStructuredJSON verifies PRD 002: gNMI Sync uses
// ExpandAndConvertIntent with Owner "running"; union member attribution on that path must match
// structured JSON import for the same owner/priority (branch pattern parity).
func TestValidate_UnionPattern_RunningExpandMatchesStructuredJSON(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "union string branch pattern matches",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantErrors: 0,
		},
		{
			name: "union string branch pattern violation",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hello AB"},
			},
			wantErrors: 1,
		},
		{
			name: "union uint32 branch has no pattern",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_Uint32{Uint32: 42},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootJSON, poolJSON := importDeviceJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, tt.req)
			resultJSON, _ := validation.Validate(ctx, rootJSON.Entry, validationConfig, poolJSON)
			if len(resultJSON.ErrorsStr()) != tt.wantErrors {
				t.Fatalf("structured JSON import (running): want %d errors, got %d: %v",
					tt.wantErrors, len(resultJSON.ErrorsStr()), resultJSON.ErrorsStr())
			}

			rootExpand, poolExpand := importDeviceExpandJSONWithOwnerPrio(t, ctx, scb, consts.RunningIntentName, consts.RunningValuesPrio, tt.req)
			resultExpand, _ := validation.Validate(ctx, rootExpand.Entry, validationConfig, poolExpand)
			t.Logf("running expand validation errors:\n%s", strings.Join(resultExpand.ErrorsStr(), "\n"))

			if len(resultExpand.ErrorsStr()) != tt.wantErrors {
				t.Errorf("running expansion path: want %d errors (same as JSON import), got %d: %v",
					tt.wantErrors, len(resultExpand.ErrorsStr()), resultExpand.ErrorsStr())
			}
		})
	}
}
