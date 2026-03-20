package testhelper

import (
	"context"
	"encoding/json"
	"runtime"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

var (
	FlagsNew      *types.UpdateInsertFlags
	FlagsExisting *types.UpdateInsertFlags
	FlagsDelete   *types.UpdateInsertFlags
)

func init() {
	FlagsNew = types.NewUpdateInsertFlags()
	FlagsNew.SetNewFlag()
	FlagsExisting = types.NewUpdateInsertFlags()
	FlagsDelete = types.NewUpdateInsertFlags().SetDeleteFlag()
}

// LoadYgotStructIntoTreeRoot loads a Ygot GoStruct into a tree root entry.
// This is a test helper function that converts a Ygot struct to JSON and imports it into the tree.
// Exported for use in other test packages.
func LoadYgotStructIntoTreeRoot(ctx context.Context, gs ygot.GoStruct, root api.Entry, owner string, prio int32, nonRevertive bool, flags *types.UpdateInsertFlags) (*types.ImportStats, error) {
	jconfStr, err := ygot.EmitJSON(gs, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: true,
	})
	if err != nil {
		return nil, err
	}

	var jsonConfAny any
	err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
	if err != nil {
		return nil, err
	}

	stp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	importProcessor := processors.NewImportConfigProcessor(jsonImporter.NewJsonTreeImporter(jsonConfAny, owner, prio, nonRevertive), flags)
	err = importProcessor.Run(ctx, root, stp)

	if err != nil {
		return nil, err
	}
	return importProcessor.GetStats(), nil
}

// ExpandUpdateFromConfig converts a Ygot GoStruct (Device config) to SDCpb Updates using the provided converter.
// This is used in tests to convert configuration structures to update messages for the tree.
func ExpandUpdateFromConfig(ctx context.Context, conf *sdcio_schema.Device, converter *utils.Converter) ([]*sdcpb.Update, error) {
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
		})
}

func AddToRoot(ctx context.Context, e api.Entry, updates []*sdcpb.Update, flags *types.UpdateInsertFlags, owner string, prio int32) error {
	for _, upd := range updates {
		cacheUpd := types.NewUpdate(nil, upd.Value, prio, owner, 0)

		_, err := ops.AddUpdateRecursive(ctx, e, upd.GetPath(), cacheUpd, flags)
		if err != nil {
			return err
		}
	}
	return nil
}

// Config1 returns a test configuration with ethernet-1/1 interface, default network instance, and various test data.
// Exported for use in other test packages.
func Config1() *sdcio_schema.Device {
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
		Patterntest: ygot.String("hallo 00"),
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

// Config2 returns a test configuration with ethernet-1/2 interface and other network instance.
// Exported for use in other test packages.
func Config2() *sdcio_schema.Device {
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
		Patterntest: ygot.String("hallo 99"),
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
