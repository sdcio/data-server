package tree

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	json_importer "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestValidate_Range_SDC_Schema(t *testing.T) {

	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Error(err)
	}

	tc := NewTreeContext(scb, "owner1", pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	root, err := NewTreeRoot(ctx, tc)

	if err != nil {
		t.Error(err)
	}

	config := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/15": {
				Name:        ygot.String("ethernet-1/15"),
				Description: ygot.String("testinterface"),
				Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
					9999: {
						Index:       ygot.Uint32(9999),
						Description: ygot.String("subif 9999"),
					},
					10000: {
						Index:       ygot.Uint32(10000),
						Description: ygot.String("subif 10000"),
					},
				},
			},
		},
	}

	jsonConfString, err := ygot.EmitJSON(config, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		t.Error(err)
	}

	var jsonConfig any
	err = json.Unmarshal([]byte(jsonConfString), &jsonConfig)
	if err != nil {
		t.Error(err)
	}

	jimporter := json_importer.NewJsonTreeImporter(jsonConfig, "owner1", 5, false)

	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool)
	if err != nil {
		t.Error(err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}

	valConf := validationConfig.DeepCopy()

	validationResult, _ := root.Validate(ctx, valConf, sharedPool)

	t.Logf("Validation Errors:\n%s", strings.Join(validationResult.ErrorsStr(), "\n"))
	t.Log(root.String())

	if len(validationResult.ErrorsStr()) != 1 {
		t.Errorf("expected %d error, got %d", 1, len(validationResult.ErrorsStr()))
	}

}

func TestValidate_RangesSigned(t *testing.T) {

	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "One",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(-1004546),
			},
			wantErrors: 1,
		},
		{
			name: "two",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(0),
			},
			wantErrors: 1,
		},
		{
			name: "three",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(-15),
			},
			wantErrors: 0,
		},
		{
			name: "four",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(1004546),
			},
			wantErrors: 1,
		},
		{
			name: "five",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(300),
			},
			wantErrors: 0,
		},
		{
			name: "six",
			req: &sdcio_schema.Device{
				Rangetestsigned: ygot.Int32(-50),
			},
			wantErrors: 0,
		},
	}

	// the test context
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	// the sdcio schema client bound
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Error(err)
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			// the tree context
			tc := NewTreeContext(scb, "owner1", pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			// the tree root
			root, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Error(err)
			}

			// the json config as string
			config, err := ygot.EmitJSON(tt.req, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
			if err != nil {
				t.Error(err)
			}

			// unmarshall into anonymous struct
			var jsonConfig any
			err = json.Unmarshal([]byte(config), &jsonConfig)
			if err != nil {
				t.Error(err)
			}

			// new json tree importer
			jimporter := json_importer.NewJsonTreeImporter(jsonConfig, "owner1", 5, false)

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

			// import via importer
			_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool)

			if err != nil {
				t.Error(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			validationResult, _ := root.Validate(ctx, validationConfig, sharedPool)

			t.Logf("Validation Errors:\n%s", strings.Join(validationResult.ErrorsStr(), "\n"))
			t.Log(root.String())

			if len(validationResult.ErrorsStr()) != tt.wantErrors {
				t.Errorf("expected %d error, got %d", tt.wantErrors, len(validationResult.ErrorsStr()))
			}

		})
	}
}

func TestValidate_RangesUnSigned(t *testing.T) {

	tests := []struct {
		name       string
		req        *sdcio_schema.Device
		wantErrors int
	}{
		{
			name: "One",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(0),
			},
			wantErrors: 1,
		},
		{
			name: "two",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(30),
			},
			wantErrors: 0,
		},
		{
			name: "three",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(4000),
			},
			wantErrors: 1,
		},
		{
			name: "four",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(5010),
			},
			wantErrors: 0,
		},
		{
			name: "five",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(9999),
			},
			wantErrors: 0,
		},
		{
			name: "six",
			req: &sdcio_schema.Device{
				Rangetestunsigned: ygot.Uint32(10000),
			},
			wantErrors: 1,
		},
		{
			name: "Leaflist - One",
			req: &sdcio_schema.Device{
				RangetestLeaflist: []uint32{
					*ygot.Uint32(0), *ygot.Uint32(25), *ygot.Uint32(500), *ygot.Uint32(5011), *ygot.Uint32(80000),
				},
			},
			wantErrors: 3,
		},
		{
			name: "Leaflist - two",
			req: &sdcio_schema.Device{
				RangetestLeaflist: []uint32{
					*ygot.Uint32(12), *ygot.Uint32(300), *ygot.Uint32(5010), *ygot.Uint32(9999),
				},
			},
			wantErrors: 0,
		},
	}

	// the test context
	ctx := context.TODO()

	mockCtrl := gomock.NewController(t)

	// the sdcio schema client bound
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Error(err)
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			// the tree context
			tc := NewTreeContext(scb, "owner1", pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			// the tree root
			root, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Error(err)
			}

			// the json config as string
			config, err := ygot.EmitJSON(tt.req, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
			if err != nil {
				t.Error(err)
			}

			// unmarshall into anonymous struct
			var jsonConfig any
			err = json.Unmarshal([]byte(config), &jsonConfig)
			if err != nil {
				t.Error(err)
			}

			// new json tree importer
			jimporter := json_importer.NewJsonTreeImporter(jsonConfig, "owner1", 5, false)

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			// import via importer
			_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jimporter, types.NewUpdateInsertFlags(), sharedPool)

			if err != nil {
				t.Error(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			// run validation
			validationResults, _ := root.Validate(ctx, validationConfig, sharedPool)

			t.Logf("Validation Errors:\n%s", strings.Join(validationResults.ErrorsStr(), "\n"))
			t.Log(root.String())

			if len(validationResults.ErrorsStr()) != tt.wantErrors {
				t.Errorf("expected %d error, got %d", tt.wantErrors, len(validationResults.ErrorsStr()))
			}

		})
	}
}
