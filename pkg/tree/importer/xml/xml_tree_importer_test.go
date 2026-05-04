package xml

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/beevik/etree"
	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestXmlTreeImporter(t *testing.T) {

	tests := []struct {
		name  string
		input string
	}{
		{
			name: "one",
			input: `<choices>
  <case1>
    <case-elem>
      <elem>foocaseval</elem>
    </case-elem>
  </case1>
</choices>
<emptyconf/>
<interface>
  <admin-state>enable</admin-state>
  <description>Foo</description>
  <name>ethernet-1/1</name>
  <subinterface>
    <description>Subinterface 0</description>
    <index>0</index>
    <type>sdcio_model_common:routed</type>
  </subinterface>
</interface>
<interface>
  <admin-state>enable</admin-state>
  <description>Foo</description>
  <name>ethernet-1/2</name>
  <subinterface>
    <description>Subinterface 0</description>
    <index>0</index>
    <type>sdcio_model_common:routed</type>
  </subinterface>
</interface>
<leaflist>
  <entry>foo</entry>
  <entry>bar</entry>
</leaflist>
<network-instance>
  <admin-state>disable</admin-state>
  <description>Default NI</description>
  <name>default</name>
  <protocol>
    <bgp/>
  </protocol>
  <type>sdcio_model_ni:default</type>
</network-instance>
<patterntest>hallo DU</patterntest>
	`,
		},
	}

	// create a gomock controller
	controller := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, controller)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			inputDoc := etree.NewDocument()
			if err := inputDoc.ReadFromString(tt.input); err != nil {
				t.Fatal(err)
			}

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

			_, err = root.ImportConfig(ctx, nil, NewXmlTreeImporter(&inputDoc.Element, "owner1", 5, false), types.NewUpdateInsertFlags(), sharedPool)
			sharedPool.CloseForSubmit()
			sharedPool.Wait()

			if err != nil {
				t.Fatal(err)
			}
			t.Log(root.String())
			fmt.Println(root.String())

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			result, err := ops.ToXML(ctx, root.Entry, false, false, false, false)
			if err != nil {
				t.Fatal(err)
			}

			utils.XmlRecursiveSortElementsByTagName(&inputDoc.Element)
			utils.XmlRecursiveSortElementsByTagName(&result.Element)

			result.Indent(2)
			xmlResultStr, err := result.WriteToString()
			if err != nil {
				t.Fatal(err)
			}

			inputDoc.Indent(2)
			inputDocStr, err := inputDoc.WriteToString()
			if err != nil {
				t.Fatal(err)
			}

			t.Log(string(xmlResultStr))

			if diff := cmp.Diff(inputDocStr, string(xmlResultStr)); diff != "" {
				t.Fatalf("Integrating xml failed. mismatch (-want +got).\nDiff:\n%s", diff)
			}
		})
	}
}

func TestXmlTreeImporterElement_IdentityRef(t *testing.T) {
	d := &sdcio_schema.Device{
		Identityref: &sdcio_schema.SdcioModel_Identityref{
			CryptoA: sdcio_schema.SdcioModelIdentityBase_CryptoAlg_des,
		},
		Intentityrefkey: map[sdcio_schema.E_SdcioModelIdentityBase_CryptoAlg]*sdcio_schema.SdcioModel_Intentityrefkey{
			sdcio_schema.SdcioModelIdentityBase_CryptoAlg_des: &sdcio_schema.SdcioModel_Intentityrefkey{
				Crypto:      sdcio_schema.SdcioModelIdentityBase_CryptoAlg_des,
				Description: ygot.String("DES crypto"),
			},
		},
	}

	// create a gomock controller
	controller := gomock.NewController(t)

	scb, err := testhelper.GetSchemaClientBound(t, controller)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))

	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	confStr, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: false,
	})
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	var v any
	json.Unmarshal([]byte(confStr), &v)

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
	if err != nil {
		t.Fatalf("failed to import test config: %v", err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}

	t.Log(root.String())

	result, err := ops.ToXML(ctx, root.Entry, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}

	result.Indent(2)
	xmlResultStr, err := result.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(xmlResultStr)

	vpf = pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc = tree.NewTreeContext(scb, vpf)

	newroot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	_, err = newroot.ImportConfig(ctx, &sdcpb.Path{}, NewXmlTreeImporter(&result.Element, consts.RunningIntentName, consts.RunningValuesPrio, false), types.NewUpdateInsertFlags(), vpf)
	if err != nil {
		t.Fatalf("failed to import test config: %v", err)
	}

	err = newroot.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}

	t.Log(newroot.String())

	if diff := cmp.Diff(root.String(), newroot.String()); diff != "" {
		t.Fatalf("Integrating xml failed. mismatch (-want +got).\nDiff:\n%s", diff)
	}
}

func TestXmlTreeImporterElement_GetTVValue_MatchedType(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		xmlText         string
		slt             *sdcpb.SchemaLeafType
		wantMatchedType string // expected matched branch TypeName ("" = nil expected)
		wantErr         bool
	}{
		{
			name:            "non-union returns input type",
			xmlText:         "hello",
			slt:             &sdcpb.SchemaLeafType{Type: "string"},
			wantMatchedType: "string",
		},
		{
			name:    "union matched string branch",
			xmlText: "hello",
			slt: &sdcpb.SchemaLeafType{
				Type: "union",
				UnionTypes: []*sdcpb.SchemaLeafType{
					{Type: "uint32"},
					{Type: "string"},
				},
			},
			wantMatchedType: "string",
		},
		{
			name:    "union matched uint32 branch",
			xmlText: "42",
			slt: &sdcpb.SchemaLeafType{
				Type: "union",
				UnionTypes: []*sdcpb.SchemaLeafType{
					{Type: "uint32"},
					{Type: "string"},
				},
			},
			wantMatchedType: "uint32",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := etree.NewDocument()
			doc.ReadFromString("<leaf>" + tt.xmlText + "</leaf>")
			elem := NewXmlTreeImporterElement(doc.Root())

			_, matchedType, err := elem.GetTVValue(ctx, tt.slt)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetTVValue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantMatchedType == "" {
				if matchedType != nil {
					t.Errorf("expected nil matchedType, got %v", matchedType)
				}
			} else {
				if matchedType == nil || matchedType.Type != tt.wantMatchedType {
					t.Errorf("matchedType = %v, want type %q", matchedType, tt.wantMatchedType)
				}
			}
		})
	}
}
