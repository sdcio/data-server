package xml

import (
	"context"
	"fmt"
	"testing"

	"github.com/beevik/etree"
	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
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

	tc := tree.NewTreeContext(scb, "test")

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

			err = root.ImportConfig(ctx, nil, NewXmlTreeImporter(&inputDoc.Element), "owner1", 5, types.NewUpdateInsertFlags())
			if err != nil {
				t.Fatal(err)
			}
			t.Log(root.String())
			fmt.Println(root.String())

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			result, err := root.ToXML(false, false, false, false)
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
