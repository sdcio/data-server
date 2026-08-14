package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	"go.uber.org/mock/gomock"
)

func TestValidate_LeafRef_UnionOptionalInstance(t *testing.T) {
	tests := []struct {
		name         string
		req          *sdcio_schema.Device
		wantErrors   int
		wantWarnings int
	}{
		{
			name: "optional union leafref - missing target issues warning not error",
			req: &sdcio_schema.Device{
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name: "optional union leafref - target exists, no warning",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {
						Name: ygot.String("ethernet-1/1"),
					},
				},
				Unionoptionalleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors:   0,
			wantWarnings: 0,
		},
	}

	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	valConf := config.NewValidationConfig()
	valConf.DisabledValidators.DisableAll()
	valConf.DisabledValidators.Leafref = false
	valConf.SetDisableConcurrency(true)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, sharedPool := importDeviceJSON(t, ctx, scb, tt.req)

			validationResult, _ := validation.Validate(ctx, root.Entry, valConf, sharedPool)

			t.Logf("Errors:\n%s", strings.Join(validationResult.ErrorsStr(), "\n"))
			t.Logf("Warnings:\n%s", strings.Join(validationResult.WarningsStr(), "\n"))

			if len(validationResult.ErrorsStr()) != tt.wantErrors {
				t.Errorf("errors: want %d, got %d: %v", tt.wantErrors, len(validationResult.ErrorsStr()), validationResult.ErrorsStr())
			}
			if len(validationResult.WarningsStr()) != tt.wantWarnings {
				t.Errorf("warnings: want %d, got %d: %v", tt.wantWarnings, len(validationResult.WarningsStr()), validationResult.WarningsStr())
			}
		})
	}
}
