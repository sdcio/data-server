package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	"go.uber.org/mock/gomock"
)

func TestValidate_Range_Union(t *testing.T) {
	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "union leaf - uint32 branch value within range",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 100},
			},
			wantErrors: 0,
		},
		{
			name: "union leaf - uint32 branch value out of range",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 5},
			},
			wantErrors: 1,
		},
		{
			name: "union leaf - string branch has no range constraint",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_String{String: "hello"},
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
