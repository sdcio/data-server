package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/utils"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func TestToJsonTable(t *testing.T) {

	var tests = []struct {
		name             string
		ietf             bool
		onlyNewOrUpdated bool
		existingConfig   func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		newConfig        func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		expected         string
	}{
		{
			name:             "JSON All",
			ietf:             false,
			onlyNewOrUpdated: false,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
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
  "patterntest": "foo"
}`,
		},
		{
			name:             "JsonIETF All",
			ietf:             true,
			onlyNewOrUpdated: false,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model:patterntest": "foo",
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
          "type": "routed"
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
      "type": "default"
    }
  ]
}`,
		},
		{
			name:             "JSON NewOrUpdated - no new",
			ietf:             false,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},

			expected: `{}`,
		},
		{
			name:             "JSON_IETF NewOrUpdated - no new",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},

			expected: `{}`,
		},
		{
			name:             "JSON NewOrUpdated - with new",
			ietf:             false,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config2()
				return expandUpdateFromConfig(ctx, c, converter)
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
  "patterntest": "bar"
}`,
		},
		{
			name:             "JSON_IETF NewOrUpdated - with new",
			ietf:             true,
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config2()
				return expandUpdateFromConfig(ctx, c, converter)
			},
			expected: `{
  "sdcio_model:patterntest": "bar",
  "sdcio_model_if:interface": [
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
  "sdcio_model_ni:network-instance": [
    {
      "admin-state": "enable",
      "description": "Other NI",
      "name": "other",
      "type": "ip-vrf"
    }
  ]
}`,
		},
		{
			name:             "XML - presence",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
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
				c := config1()
				upds, err := expandUpdateFromConfig(ctx, c, converter)
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
				c := config1()
				return expandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := config1()

				upds, err := expandUpdateFromConfig(ctx, c, converter)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scb, err := getSchemaClientBound(t)
			if err != nil {
				t.Fatal(err)
			}

			owner := "owner1"

			ctx := context.Background()

			tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), owner)
			root, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			converter := utils.NewConverter(scb)

			updsExisting, err := tt.existingConfig(ctx, converter)
			if err != nil {
				t.Error(err)
			}

			err = addToRoot(ctx, root, updsExisting, false, owner)
			if err != nil {
				t.Fatal(err)
			}

			if tt.newConfig != nil {
				updsNew, err := tt.newConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}
				err = addToRoot(ctx, root, updsNew, true, owner)
				if err != nil {
					t.Fatal(err)
				}
			}
			root.FinishInsertionPhase()

			var jsonStruct any

			if tt.ietf {
				jsonStruct, err = root.ToJsonIETF(tt.onlyNewOrUpdated)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				jsonStruct, err = root.ToJson(tt.onlyNewOrUpdated)
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
		})
	}
}

func config1() *sdcio_schema.Device {
	return &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
				Description: ygot.String("Foo"),
				Name:        ygot.String("ethernet-1/1"),
				Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
					0: {
						Description: ygot.String("Subinterface 0"),
						Type:        sdcio_schema.SdcioModelCommon_SiType_routed,
						Index:       ygot.Uint32(0),
					},
				},
			},
		},
		Choices: &sdcio_schema.SdcioModel_Choices{
			Case1: &sdcio_schema.SdcioModel_Choices_Case1{
				CaseElem: &sdcio_schema.SdcioModel_Choices_Case1_CaseElem{
					Elem: ygot.String("foocaseval"),
				},
			},
		},
		Leaflist: &sdcio_schema.SdcioModel_Leaflist{
			Entry: []string{
				"foo",
				"bar",
			},
		},
		Patterntest: ygot.String("foo"),
		NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
			"default": {
				AdminState:  sdcio_schema.SdcioModelNi_AdminState_disable,
				Description: ygot.String("Default NI"),
				Type:        sdcio_schema.SdcioModelNi_NiType_default,
				Name:        ygot.String("default"),
			},
		},
	}
}

func config2() *sdcio_schema.Device {
	return &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/2": {
				AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
				Description: ygot.String("Foo"),
				Name:        ygot.String("ethernet-1/2"),
				Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
					5: {
						Description: ygot.String("Subinterface 5"),
						Type:        sdcio_schema.SdcioModelCommon_SiType_routed,
						Index:       ygot.Uint32(5),
					},
				},
			},
		},
		Patterntest: ygot.String("bar"),
		NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
			"other": {
				AdminState:  sdcio_schema.SdcioModelNi_AdminState_enable,
				Description: ygot.String("Other NI"),
				Type:        sdcio_schema.SdcioModelNi_NiType_ip_vrf,
				Name:        ygot.String("other"),
			},
		},
	}

}

func expandUpdateFromConfig(ctx context.Context, conf *sdcio_schema.Device, converter *utils.Converter) ([]*sdcpb.Update, error) {
	if conf == nil {
		return nil, nil
	}

	strJson, err := ygot.EmitJSON(conf, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: true,
	})
	if err != nil {
		return nil, err
	}

	return converter.ExpandUpdate(ctx,
		&sdcpb.Update{
			Path: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{},
			},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(strJson)}},
		},
		true)
}

func addToRoot(ctx context.Context, root *RootEntry, updates []*sdcpb.Update, isNew bool, owner string) error {
	for _, upd := range updates {
		b, err := proto.Marshal(upd.Value)
		if err != nil {
			return err
		}
		cacheUpd, err := cache.NewUpdate(utils.ToStrings(upd.GetPath(), false, false), b, 5, owner, 0), nil
		if err != nil {
			return err
		}

		_, err = root.AddCacheUpdateRecursive(ctx, cacheUpd, isNew)
		if err != nil {
			return err
		}
	}
	return nil
}
