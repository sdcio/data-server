package ops_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockschemaclientbound"
	"github.com/sdcio/data-server/pkg/pool"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// newSchemaClientMarkingSensitive returns a mock schema client that behaves like
// the standard test schema client but marks the leaf named sensitiveLeafName as
// Sensitive=true in its returned schema.
func newSchemaClientMarkingSensitive(t *testing.T, ctrl *gomock.Controller, sensitiveLeafName string) *mockschemaclientbound.MockSchemaClientBound {
	t.Helper()
	scb, err := testhelper.GetSchemaClientBoundMarkingLeafSensitive(t, ctrl, sensitiveLeafName)
	if err != nil {
		t.Fatal(err)
	}
	return scb
}

func TestToJsonTable(t *testing.T) {

	var tests = []struct {
		name             string
		ietf             bool
		onlyNewOrUpdated bool
		existingConfig   func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		runningConfig    func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		newConfig        func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		expected         string
	}{
		{
			name:             "JSON All",
			ietf:             false,
			onlyNewOrUpdated: false,
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "choices": {
    "case1": {
      "case-elem": {
        "elem": "foocaseval"
      }
    }
  },
  "interface": [
    {
      "admin-state": "enable",
      "description": "Foo",
      "name": "ethernet-1/1",
      "subinterface": [
        {
          "description": "Subinterface 0",
          "index": 0,
          "type": "routed"
        }
      ]
    }
  ],
  "leaflist": {
    "entry": [
      "foo",
      "bar"
    ]
  },
  "network-instance": [
    {
      "admin-state": "disable",
      "description": "Default NI",
      "name": "default",
      "type": "default"
    }
  ],
  "patterntest": "hallo 00"
}`,
		},
		{
			name:             "JsonIETF All",
			ietf:             true,
			onlyNewOrUpdated: false,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model:patterntest": "hallo 00",
  "sdcio_model_choice:choices": {
    "case1": {
      "case-elem": {
        "elem": "foocaseval"
      }
    }
  },
  "sdcio_model_if:interface": [
    {
      "admin-state": "enable",
      "description": "Foo",
      "name": "ethernet-1/1",
      "subinterface": [
        {
          "description": "Subinterface 0",
          "index": 0,
          "type": "sdcio_model_common:routed"
        }
      ]
    }
  ],
  "sdcio_model_leaflist:leaflist": {
    "entry": [
      "foo",
      "bar"
    ]
  },
  "sdcio_model_ni:network-instance": [
    {
      "admin-state": "disable",
      "description": "Default NI",
      "name": "default",
      "type": "sdcio_model_ni:default"
    }
  ]
}`,
		},
		{
			name:             "JSON NewOrUpdated - no new",
			ietf:             false,
			onlyNewOrUpdated: true,
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},

			expected: `null`,
		},
		{
			name:             "JSON_IETF NewOrUpdated - no new",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},

			expected: `null`,
		},
		{
			name:             "JSON NewOrUpdated - with new",
			ietf:             false,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config2()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "interface": [
    {
      "admin-state": "enable",
      "description": "Foo",
      "name": "ethernet-1/2",
      "subinterface": [
        {
          "description": "Subinterface 5",
          "index": 5,
          "type": "routed"
        }
      ]
    }
  ],
  "network-instance": [
    {
      "admin-state": "enable",
      "description": "Other NI",
      "name": "other",
      "type": "ip-vrf"
    }
  ],
  "patterntest": "hallo 99"
}`,
		},
		{
			name:             "JSON_IETF NewOrUpdated - with new",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config2()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model:patterntest": "hallo 99",
  "sdcio_model_if:interface": [
    {
      "admin-state": "enable",
      "description": "Foo",
      "name": "ethernet-1/2",
      "subinterface": [
        {
          "description": "Subinterface 5",
          "index": 5,
          "type": "sdcio_model_common:routed"
        }
      ]
    }
  ],
  "sdcio_model_ni:network-instance": [
    {
      "admin-state": "enable",
      "description": "Other NI",
      "name": "other",
      "type": "sdcio_model_ni:ip-vrf"
    }
  ]
}`,
		},
		{
			name:             "JSON_IETF - int16",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				c.Interface["ethernet-1/1"].Mtu = ygot.Uint16(1500)
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model_if:interface": [
    {
      "mtu": 1500,
      "name": "ethernet-1/1"
    }
  ]
}`,
		},
		{
			name:             "JSON - presence",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "network-instance": [
    {
      "name": "default",
      "protocol": {
        "bgp": {}
      }
    }
  ]
}`,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				upds, err := testhelper.ExpandUpdateFromConfig(ctx, c, converter)
				if err != nil {
					return nil, err
				}
				upds = append(upds, &sdcpb.Update{
					Path: &sdcpb.Path{
						Elem: []*sdcpb.PathElem{
							{Name: "network-instance", Key: map[string]string{"name": "default"}},
							{Name: "protocol"},
							{Name: "bgp"},
						},
					},
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}},
				})
				return upds, nil
			},
		},
		{
			name:             "JSON - empty",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()

				upds, err := testhelper.ExpandUpdateFromConfig(ctx, c, converter)
				if err != nil {
					return nil, err
				}
				upds = append(upds, &sdcpb.Update{Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "emptyconf"}}}, Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}}})
				return upds, nil
			},
			expected: `{
  "emptyconf": {}
}`,
		},
		{
			name:             "JSON_IETF - identityref",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				c.Identityref = &sdcio_schema.SdcioModel_Identityref{
					CryptoA: sdcio_schema.SdcioModelIdentityBase_CryptoAlg_des3,
					CryptoB: sdcio_schema.SdcioModelIdentityBase_CryptoAlg_otherAlgo,
				}
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model_identity:identityref": {
    "cryptoA": "sdcio_model_identity_types:des3",
    "cryptoB": "sdcio_model_identity:otherAlgo"
  }
}`,
		},
	}

	flagsNew := types.NewUpdateInsertFlags()
	flagsNew.SetNewFlag()

	flagsOld := types.NewUpdateInsertFlags()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}

			owner := "owner1"

			ctx := context.Background()

			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			converter := utils.NewConverter(scb)

			if tt.runningConfig != nil {
				updsRunning, err := tt.runningConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}

				err = testhelper.AddToRoot(ctx, root.Entry, updsRunning, flagsOld, consts.RunningIntentName, consts.RunningValuesPrio)
				if err != nil {
					t.Fatal(err)
				}
			}

			updsExisting, err := tt.existingConfig(ctx, converter)
			if err != nil {
				t.Error(err)
			}

			err = testhelper.AddToRoot(ctx, root.Entry, updsExisting, flagsOld, owner, 5)
			if err != nil {
				t.Fatal(err)
			}

			if tt.newConfig != nil {
				updsNew, err := tt.newConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}
				err = testhelper.AddToRoot(ctx, root.Entry, updsNew, flagsNew, owner, 5)
				if err != nil {
					t.Fatal(err)
				}
			}
			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			fmt.Println(root.String())

			var jsonStruct any

			renderOpts := ops.RenderOpts{OnlyNewOrUpdated: tt.onlyNewOrUpdated}
			if tt.ietf {
				jsonStruct, err = ops.ToJsonIETF(ctx, root.Entry, renderOpts)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				jsonStruct, err = ops.ToJson(ctx, root.Entry, renderOpts)
				if err != nil {
					t.Fatal(err)
				}
			}

			jsonStr, err := json.MarshalIndent(jsonStruct, "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(string(jsonStr))

			if diff := cmp.Diff(tt.expected, string(jsonStr)); diff != "" {
				var ietfStr = ""
				if tt.ietf {
					ietfStr = "IETF"
				}
				t.Fatalf("ToJson%s() failed.\nDiff:\n%s", ietfStr, diff)
			}

			mockCtrl.Finish()
		})
	}
}

func TestToJsonSensitiveRedaction(t *testing.T) {
	flagsOld := types.NewUpdateInsertFlags()

	tests := []struct {
		name            string
		exposeSensitive bool
		wantPatterntest string
	}{
		{
			name:            "redacts sensitive leaf when IncludeSensitive=false",
			exposeSensitive: false,
			wantPatterntest: "***",
		},
		{
			name:            "reveals sensitive leaf when IncludeSensitive=true",
			exposeSensitive: true,
			wantPatterntest: "hallo 00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			scb := newSchemaClientMarkingSensitive(t, ctrl, "patterntest")

			ctx := context.Background()
			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			converter := utils.NewConverter(scb)

			upds, err := testhelper.ExpandUpdateFromConfig(ctx, testhelper.Config1(), converter)
			if err != nil {
				t.Fatal(err)
			}
			if err := testhelper.AddToRoot(ctx, root.Entry, upds, flagsOld, "owner1", 5); err != nil {
				t.Fatal(err)
			}
			if err := root.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			jsonStruct, err := ops.ToJson(ctx, root.Entry, ops.RenderOpts{IncludeSensitive: tt.exposeSensitive})
			if err != nil {
				t.Fatal(err)
			}

			jsonBytes, err := json.Marshal(jsonStruct)
			if err != nil {
				t.Fatal(err)
			}

			var result map[string]any
			if err := json.Unmarshal(jsonBytes, &result); err != nil {
				t.Fatal(err)
			}

			got, ok := result["patterntest"].(string)
			if !ok {
				t.Fatalf("patterntest not found or not a string in JSON output: %s", string(jsonBytes))
			}
			if diff := cmp.Diff(tt.wantPatterntest, got); diff != "" {
				t.Errorf("patterntest mismatch (-want +got):\n%s", diff)
			}

			ctrl.Finish()
		})
	}
}
