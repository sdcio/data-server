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

// TestValidate_Union_XMLCanonicalReimportParityWithJSON asserts that configuration
// loaded through the XML tree importer after ops.ToXML (canonical device XML from a
// JSON-imported tree) yields the same validation error counts as direct JSON import
// for logically equivalent values where XML text parsing accepts the update (PRD
// 006 / issue 006).
//
// Cases where no union branch accepts the lexical form fail at XML import
// (TVFromStringWithType) rather than load-then-validate; those are covered separately
// and documented under issue 006 — they are not comparable to JSON+ygot paths that
// may still materialize a TypedValue and fail in validation.
func TestValidate_Union_XMLCanonicalReimportParityWithJSON(t *testing.T) {
	tests := []struct {
		name           string
		req            *sdcio_schema.Device
		wantAfterJSON  int
		wantAfterXMLRI int
	}{
		{
			name: "valid union pattern value survives XML re-import",
			req: &sdcio_schema.Device{
				Unionpatterntest: &sdcio_schema.SdcioModel_Unionpatterntest_Union_String{String: "hallo AB"},
			},
			wantAfterJSON:  0,
			wantAfterXMLRI: 0,
		},
		{
			name: "union range string branch same outcome after XML re-import",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_String{String: "hello"},
			},
			wantAfterJSON:  0,
			wantAfterXMLRI: 0,
		},
		{
			name: "union range uint32 in-range same outcome after XML re-import",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 100},
			},
			wantAfterJSON:  0,
			wantAfterXMLRI: 0,
		},
		{
			name: "range-violating union uint32 still errors after XML re-import",
			req: &sdcio_schema.Device{
				Unionrangetest: &sdcio_schema.SdcioModel_Unionrangetest_Union_Uint32{Uint32: 500},
			},
			wantAfterJSON:  1,
			wantAfterXMLRI: 1,
		},
		{
			name: "union leafref branch resolves same after XML re-import",
			req: &sdcio_schema.Device{
				Interface: map[string]*sdcio_schema.SdcioModel_Interface{
					"ethernet-1/1": {
						Name: ygot.String("ethernet-1/1"),
					},
				},
				Unionleafreftest: ygot.String("ethernet-1/1"),
			},
			wantAfterJSON:  0,
			wantAfterXMLRI: 0,
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
			rootJSON, poolJSON := importDeviceJSON(t, ctx, scb, tt.req)
			resJSON, _ := validation.Validate(ctx, rootJSON.Entry, validationConfig, poolJSON)
			t.Logf("Validation errors after JSON import:\n%s", strings.Join(resJSON.ErrorsStr(), "\n"))
			if got := len(resJSON.ErrorsStr()); got != tt.wantAfterJSON {
				t.Fatalf("after JSON import: want %d error(s), got %d: %v",
					tt.wantAfterJSON, got, resJSON.ErrorsStr())
			}

			rootXML, poolXML := importDeviceJSONThenCanonicalXMLReimport(t, ctx, scb, tt.req)
			resXML, _ := validation.Validate(ctx, rootXML.Entry, validationConfig, poolXML)
			t.Logf("Validation errors after XML re-import:\n%s", strings.Join(resXML.ErrorsStr(), "\n"))
			if got := len(resXML.ErrorsStr()); got != tt.wantAfterXMLRI {
				t.Errorf("after XML re-import: want %d error(s), got %d: %v",
					tt.wantAfterXMLRI, got, resXML.ErrorsStr())
			}
		})
	}
}
