package validation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"

	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// buildTree creates a tree populated with config supplied as a plain-JSON map.
func buildTree(t *testing.T, jsonConf map[string]any) *tree.RootEntry {
	t.Helper()
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(jsonConf, "owner1", 500, false), types.NewUpdateInsertFlags(), vpf)
	if err != nil {
		t.Fatal(err)
	}

	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestLeafref_IdentityrefKey verifies that leafref validation resolves
// current()-relative key predicates whose values are identityref leaves.
// Without the tv.ToString() fix in resolveLeafrefKeyPath the key is "" and
// the reference cannot be found.
func TestLeafref_IdentityrefKey(t *testing.T) {
	tests := []struct {
		name        string
		conf        map[string]any
		wantErrLen  int
	}{
		{
			name: "identityref key resolves - pass",
			conf: map[string]any{
				"identityref":        map[string]any{"cryptoA": "otherAlgo"},
				"intentityrefkey":    []any{map[string]any{"crypto": "otherAlgo", "description": "my-desc"}},
				"intentityrefkey-ref": "my-desc",
			},
			wantErrLen: 0,
		},
		{
			name: "identityref key mismatch - fail",
			conf: map[string]any{
				"identityref":        map[string]any{"cryptoA": "rsa"},
				"intentityrefkey":    []any{map[string]any{"crypto": "otherAlgo", "description": "my-desc"}},
				"intentityrefkey-ref": "my-desc",
			},
			wantErrLen: 1,
		},
	}

	lrefPath := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("intentityrefkey-ref", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			root := buildTree(t, tt.conf)

			e, err := ops.NavigateSdcpbPath(ctx, root.Entry, lrefPath)
			if err != nil {
				t.Fatalf("NavigateSdcpbPath: %v", err)
			}

			valConf := config.NewValidationConfig()
			valConf.DisabledValidators.DisableAll()
			valConf.DisabledValidators.Leafref = false
			valConf.DisableConcurrency = true

			result, _ := validation.Validate(ctx, e, valConf, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			if len(result) != tt.wantErrLen {
				t.Errorf("Validate() returned %d errors, want %d: %v", len(result), tt.wantErrLen, result)
			}
		})
	}
}

// TestMust_IdentityrefKey verifies that must-statement validation navigates
// correctly when the predicate key value originates from an identityref leaf.
// Without the p.StripPathElemPrefixPath() fix in Navigate the YangString()
// prefix ("sdcio_identity:otherAlgo") prevents the list entry from being found.
func TestMust_IdentityrefKey(t *testing.T) {
	mustPath := &sdcpb.Path{
		Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("identityref-must-test", nil)},
	}

	tests := []struct {
		name       string
		conf       map[string]any
		wantErrLen int
	}{
		{
			name: "must satisfied - pass",
			conf: map[string]any{
				"identityref-must-test": map[string]any{"crypto-ref": "otherAlgo"},
				"intentityrefkey":       []any{map[string]any{"crypto": "otherAlgo", "description": "present"}},
			},
			wantErrLen: 0,
		},
		{
			name: "must not satisfied - fail",
			conf: map[string]any{
				"identityref-must-test": map[string]any{"crypto-ref": "otherAlgo"},
				// no intentityrefkey entry → description path resolves to "" → must false
			},
			wantErrLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			root := buildTree(t, tt.conf)

			e, err := ops.NavigateSdcpbPath(ctx, root.Entry, mustPath)
			if err != nil {
				t.Fatalf("NavigateSdcpbPath: %v", err)
			}

			valConf := config.NewValidationConfig()
			valConf.DisabledValidators.DisableAll()
			valConf.DisabledValidators.MustStatement = false
			valConf.DisableConcurrency = true

			result, _ := validation.Validate(ctx, e, valConf, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			if len(result) != tt.wantErrLen {
				t.Errorf("Validate() returned %d errors, want %d: %v", len(result), tt.wantErrLen, result)
			}
		})
	}
}

func Test_sharedEntryAttributes_validateLeafRefs(t *testing.T) {
	owner1 := "owner1"

	tests := []struct {
		name              string
		ygotDevice        func() ygot.GoStruct
		lrefNodePath      *sdcpb.Path
		expectedResultLen int
	}{
		{
			name: "interface lref resolution - pass",
			ygotDevice: func() ygot.GoStruct {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
							Description: ygot.String("Test"),
							Name:        ygot.String("ethernet-1/1"),
							Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
								5: {
									Index:       ygot.Uint32(5),
									Description: ygot.String("Unit 5"),
								},
							},
						},
					},
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"ni1": {
							Name: ygot.String("ni1"),
							Type: sdcio_schema.SdcioModelNi_NiType_default,
							Interface: map[string]*sdcio_schema.SdcioModel_NetworkInstance_Interface{
								"ethernet-1/1": {
									Name: ygot.String("ethernet-1/1"),
									InterfaceRef: &sdcio_schema.SdcioModel_NetworkInstance_Interface_InterfaceRef{
										Interface:    ygot.String("ethernet-1/1"),
										Subinterface: ygot.Uint32(5),
									},
								},
							},
						},
					},
				}
				return d
			},
			lrefNodePath: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("network-instance", map[string]string{"name": "ni1"}),
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("interface-ref", nil),
					sdcpb.NewPathElem("interface", nil),
				},
			},
			expectedResultLen: 0,
		},
		{
			name: "subinterface lref resolution - pass",
			ygotDevice: func() ygot.GoStruct {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
							Description: ygot.String("Test"),
							Name:        ygot.String("ethernet-1/1"),
							Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
								5: {
									Index:       ygot.Uint32(5),
									Description: ygot.String("Unit 5"),
								},
							},
						},
					},
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"ni1": {
							Name: ygot.String("ni1"),
							Type: sdcio_schema.SdcioModelNi_NiType_default,
							Interface: map[string]*sdcio_schema.SdcioModel_NetworkInstance_Interface{
								"ethernet-1/1": {
									Name: ygot.String("ethernet-1/1"),
									InterfaceRef: &sdcio_schema.SdcioModel_NetworkInstance_Interface_InterfaceRef{
										Interface:    ygot.String("ethernet-1/1"),
										Subinterface: ygot.Uint32(5),
									},
								},
							},
						},
					},
				}
				return d
			},
			lrefNodePath: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("network-instance", map[string]string{"name": "ni1"}),
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("interface-ref", nil),
					sdcpb.NewPathElem("subinterface", nil),
				},
			},
			expectedResultLen: 0,
		},
		{
			name: "interface lref resolution - fail",
			ygotDevice: func() ygot.GoStruct {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/2": {
							AdminState: sdcio_schema.SdcioModelIf_AdminState_enable,
							Name:       ygot.String("ethernet-1/2"),
						},
					},
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"ni1": {
							Name: ygot.String("ni1"),
							Type: sdcio_schema.SdcioModelNi_NiType_default,
							Interface: map[string]*sdcio_schema.SdcioModel_NetworkInstance_Interface{
								"ethernet-1/1": {
									Name: ygot.String("ethernet-1/1"),
									InterfaceRef: &sdcio_schema.SdcioModel_NetworkInstance_Interface_InterfaceRef{
										Interface:    ygot.String("ethernet-1/1"),
										Subinterface: ygot.Uint32(5),
									},
								},
							},
						},
					},
				}
				return d
			},
			lrefNodePath: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("network-instance", map[string]string{"name": "ni1"}),
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("interface-ref", nil),
					sdcpb.NewPathElem("interface", nil),
				},
			},
			expectedResultLen: 1,
		},
		{
			name: "interface lref resolution - fail",
			ygotDevice: func() ygot.GoStruct {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							AdminState: sdcio_schema.SdcioModelIf_AdminState_enable,
							Name:       ygot.String("ethernet-1/1"),
							Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
								6: {
									Index:       ygot.Uint32(6),
									Description: ygot.String("foo"),
								},
							},
						},
					},
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"ni1": {
							Name: ygot.String("ni1"),
							Type: sdcio_schema.SdcioModelNi_NiType_default,
							Interface: map[string]*sdcio_schema.SdcioModel_NetworkInstance_Interface{
								"ethernet-1/1": {
									Name: ygot.String("ethernet-1/1"),
									InterfaceRef: &sdcio_schema.SdcioModel_NetworkInstance_Interface_InterfaceRef{
										Interface:    ygot.String("ethernet-1/1"),
										Subinterface: ygot.Uint32(5),
									},
								},
							},
						},
					},
				}
				return d
			},
			lrefNodePath: &sdcpb.Path{
				IsRootBased: true,
				Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("network-instance", map[string]string{"name": "ni1"}),
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("interface-ref", nil),
					sdcpb.NewPathElem("subinterface", nil),
				},
			},
			expectedResultLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// create a gomock controller
			controller := gomock.NewController(t)
			defer controller.Finish()

			ctx := context.Background()

			sc, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Fatal(err)
			}
			scb := schemaClient.NewSchemaClientBound(schema, sc)
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			jconfStr, err := ygot.EmitJSON(tt.ygotDevice(), &ygot.EmitJSONConfig{
				Format:         ygot.RFC7951,
				SkipValidation: true,
			})
			if err != nil {
				t.Fatal(err)
			}

			var jsonConfAny any
			err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
			if err != nil {
				t.Fatal(err)
			}

			newFlag := types.NewUpdateInsertFlags()

			vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(jsonConfAny, owner1, 500, false), newFlag, vpf)
			if err != nil {
				t.Fatal(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(root.String())

			e, err := ops.NavigateSdcpbPath(ctx, root.Entry, tt.lrefNodePath)
			if err != nil {
				t.Error(err)
			}

			// make sure we're looking at a leafref
			if e.GetSchema().GetField().GetType().GetType() != "leafref" {
				t.Fatalf("referenced field %s not a leafref, fix test.", tt.lrefNodePath.ToXPath(false))
			}

			valConf := config.NewValidationConfig()
			valConf.DisabledValidators.DisableAll()
			valConf.DisabledValidators.Leafref = false
			valConf.DisableConcurrency = true // disable concurrency for deterministic test results

			result, _ := validation.Validate(ctx, e, valConf, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			if len(result) != tt.expectedResultLen {
				t.Fatalf("expected %d, got %d errors on leafref validation", tt.expectedResultLen, len(result))
			}
		})
	}
}
