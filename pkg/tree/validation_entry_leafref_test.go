package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

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
			tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

			root, err := NewTreeRoot(ctx, tc)
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

			e, err := root.NavigateSdcpbPath(ctx, tt.lrefNodePath)
			if err != nil {
				t.Error(err)
			}

			s, ok := e.(*sharedEntryAttributes)
			if !ok {
				t.Errorf("failed to assert type *sharedEntryAttributes")
			}

			// make sure we're looking at a leafref
			if s.schema.GetField().GetType().GetType() != "leafref" {
				t.Fatalf("referenced field %s not a leafref, fix test.", tt.lrefNodePath.ToXPath(false))
			}

			resultChan := make(chan<- *types.ValidationResultEntry, 20)
			stats := types.NewValidationStats()
			s.validateLeafRefs(ctx, resultChan, stats)

			if len(resultChan) != tt.expectedResultLen {
				t.Fatalf("expected %d, got %d errors on leafref validation", tt.expectedResultLen, len(resultChan))
			}

		})
	}
}
