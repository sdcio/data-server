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

func TestValidate_LeafRef_Union(t *testing.T) {
	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "union leaf - leafref branch resolves to existing interface",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {
						Name: ygot.String("ethernet-1/1"),
					},
				},
				Unionleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors: 0,
		},
		{
			name: "union leaf - leafref branch points to nonexistent interface",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/2": {
						Name: ygot.String("ethernet-1/2"),
					},
				},
				Unionleafreftest: ygot.String("ethernet-1/1"),
			},
			wantErrors: 1,
		},
	}

	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	// validation config that only runs leafref validation
	valConf := config.NewValidationConfig()
	valConf.DisabledValidators.DisableAll()
	valConf.DisabledValidators.Leafref = false
	valConf.SetDisableConcurrency(true)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, sharedPool := importDeviceJSON(t, ctx, scb, tt.req)

			validationResult, _ := validation.Validate(ctx, root.Entry, valConf, sharedPool)

			t.Logf("Validation Errors:\n%s", strings.Join(validationResult.ErrorsStr(), "\n"))

			if len(validationResult.ErrorsStr()) != tt.wantErrors {
				t.Errorf("expected %d error(s), got %d: %v", tt.wantErrors, len(validationResult.ErrorsStr()), validationResult.ErrorsStr())
			}
		})
	}
}
