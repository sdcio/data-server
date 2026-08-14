package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	"go.uber.org/mock/gomock"
)

func TestValidate_Pattern(t *testing.T) {
	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "non-union leaf pattern matches",
			req: &sdcio_schema.Device{
				Patterntest: ygot.String("hallo AB"),
			},
			wantErrors: 0,
		},
		{
			name: "non-union leaf pattern violation",
			req: &sdcio_schema.Device{
				Patterntest: ygot.String("hello AB"),
			},
			wantErrors: 1,
		},
		{
			name: "union leaf - string branch pattern matches",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantErrors: 0,
		},
		{
			name: "union leaf - string branch pattern violation",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hello AB"},
			},
			wantErrors: 1,
		},
		{
			name: "union leaf - uint32 branch has no pattern constraint",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_Uint32{Uint32: 42},
			},
			wantErrors: 0,
		},
	}

	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, sharedPool := importDeviceJSON(t, ctx, scb, tt.req)

			validationResult, _ := validation.Validate(ctx, root.Entry, validationConfig, sharedPool)

			t.Logf("Validation Errors:\n%s", strings.Join(validationResult.ErrorsStr(), "\n"))

			if len(validationResult.ErrorsStr()) != tt.wantErrors {
				t.Errorf("expected %d error(s), got %d: %v", tt.wantErrors, len(validationResult.ErrorsStr()), validationResult.ErrorsStr())
			}
		})
	}
}
