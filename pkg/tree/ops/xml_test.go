package ops_test

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"

	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestToXMLTable(t *testing.T) {
	var tests = []struct {
		name                   string
		onlyNewOrUpdated       bool
		honorNamespace         bool
		operationWithNamespace bool
		useOperationRemove     bool
		existingConfig         func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		runningConfig          func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		newConfig              func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error)
		expected               string
		skip                   bool
	}{
		{
			name:             "XML All",
			onlyNewOrUpdated: false,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<choices>
  <case1>
    <case-elem>
      <elem>foocaseval</elem>
    </case-elem>
  </case1>
</choices>
<interface>
  <name>ethernet-1/1</name>
  <admin-state>enable</admin-state>
  <description>Foo</description>
  <subinterface>
    <index>0</index>
    <description>Subinterface 0</description>
    <type>sdcio_model_common:routed</type>
  </subinterface>
</interface>
<leaflist>
  <entry>foo</entry>
  <entry>bar</entry>
</leaflist>
<network-instance>
  <name>default</name>
  <admin-state>disable</admin-state>
  <description>Default NI</description>
  <type>sdcio_model_ni:default</type>
</network-instance>
<patterntest>hallo 00</patterntest>
`,
		},
		{
			name:             "XML - no new",
			onlyNewOrUpdated: true,
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: ``,
		},
		{
			name:             "XML NewOrUpdated - some elements deleted, some updated",
			onlyNewOrUpdated: true,
			skip:             false,
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
			expected: `<choices operation="delete"/>
<interface operation="delete">
  <name>ethernet-1/1</name>
</interface>
<interface>
  <name>ethernet-1/2</name>
  <admin-state>enable</admin-state>
  <description>Foo</description>
  <subinterface>
    <index>5</index>
    <description>Subinterface 5</description>
    <type>sdcio_model_common:routed</type>
  </subinterface>
</interface>
<leaflist operation="delete"/>
<network-instance operation="delete">
  <name>default</name>
</network-instance>
<network-instance>
  <name>other</name>
  <admin-state>enable</admin-state>
  <description>Other NI</description>
  <type>sdcio_model_ni:ip-vrf</type>
</network-instance>
<patterntest>hallo 99</patterntest>
`,
		},
		{
			name:             "XML - delete ethernet-1_1, honor namespace, operatin With namespace, remove",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<interface xmlns="urn:sdcio/model" xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove">
  <name>ethernet-1/1</name>
</interface>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				delete(c.Interface, "ethernet-1/1")
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - honor namespace, operatin With namespace",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<interface xmlns="urn:sdcio/model" xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="delete">
  <name>ethernet-1/1</name>
</interface>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     false,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				delete(c.Interface, "ethernet-1/1")
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - delete certain ethernet-1_1 attributes update another",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<interface xmlns="urn:sdcio/model">
  <name>ethernet-1/1</name>
  <description xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove"/>
  <subinterface>
    <index>0</index>
    <description xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove"/>
    <type>sdcio_model_common:bridged</type>
  </subinterface>
</interface>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				c.Interface["ethernet-1/1"].Description = nil
				c.Interface["ethernet-1/1"].Subinterface[0].Description = nil
				c.Interface["ethernet-1/1"].Subinterface[0].Type = sdcio_schema.SdcioModelCommon_SiType_bridged
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - delete ethernet-1_1 add ethernet-1_2",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			skip: false,
			expected: `<choices operation="delete"/>
<interface operation="delete">
  <name>ethernet-1/1</name>
</interface>
<interface>
  <name>ethernet-1/2</name>
  <admin-state>enable</admin-state>
  <description>Test</description>
</interface>
<patterntest operation="delete"/>
`,
			honorNamespace:         false,
			operationWithNamespace: false,
			useOperationRemove:     false,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				delete(c.Interface, "ethernet-1/1")
				c.Interface["ethernet-1/2"] = &sdcio_schema.SdcioModel_Interface{
					AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
					Name:        ygot.String("ethernet-1/2"),
					Description: ygot.String("Test"),
				}
				c.Patterntest = nil
				c.Choices.Case1.CaseElem.Elem = nil
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - replace direct leaf and choice",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<choices xmlns="urn:sdcio/model">
  <case1 xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove"/>
  <case2>
    <log>true</log>
  </case2>
</choices>
<patterntest xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove"/>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				c.Patterntest = nil
				c.Choices.Case1.CaseElem.Elem = nil
				c.Choices.Case2 = &sdcio_schema.SdcioModel_Choices_Case2{
					Log: ygot.Bool(true),
				}
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - empty",
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
			expected: `<emptyconf/>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
		},
		{
			name:             "XML - presence",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				//c := testhelper.Config1()
				c := &sdcio_schema.Device{
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"default": {
							AdminState:  sdcio_schema.SdcioModelNi_AdminState_disable,
							Description: ygot.String("Default NI"),
							Type:        sdcio_schema.SdcioModelNi_NiType_default,
							Name:        ygot.String("default"),
						},
					},
				}
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<network-instance xmlns="urn:sdcio/model">
  <name>default</name>
  <protocol>
    <bgp/>
  </protocol>
</network-instance>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				// c := testhelper.Config1()
				c := &sdcio_schema.Device{
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"default": {
							AdminState:  sdcio_schema.SdcioModelNi_AdminState_disable,
							Description: ygot.String("Default NI"),
							Type:        sdcio_schema.SdcioModelNi_NiType_default,
							Name:        ygot.String("default"),
						},
					},
				}
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				// c := testhelper.Config1()
				c := &sdcio_schema.Device{
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"default": {
							AdminState:  sdcio_schema.SdcioModelNi_AdminState_disable,
							Description: ygot.String("Default NI"),
							Type:        sdcio_schema.SdcioModelNi_NiType_default,
							Name:        ygot.String("default"),
						},
					},
				}
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
			name:             "XML - replace choice",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			expected: `<choices xmlns="urn:sdcio/model">
  <case1 xmlns:nc="urn:ietf:params:xml:ns:netconf:base:1.0" nc:operation="remove"/>
  <case2>
    <log>true</log>
  </case2>
</choices>
`,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				c.Choices.Case1 = nil
				c.Choices.Case2 = &sdcio_schema.SdcioModel_Choices_Case2{
					Log: ygot.Bool(true),
				}
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
		},
		{
			name:             "XML - device returns leaflist out of order",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				return testhelper.ExpandUpdateFromConfig(ctx, c, converter)
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				c := testhelper.Config1()
				slices.Reverse(c.Leaflist.Entry)
				upds, err := testhelper.ExpandUpdateFromConfig(ctx, c, converter)
				if err != nil {
					return nil, err
				}
				return upds, nil
			},
			expected:               ``,
			honorNamespace:         true,
			operationWithNamespace: true,
			useOperationRemove:     true,
			newConfig:              nil,
		},
		{
			name:             "XML - delete multi key item (static route)",
			onlyNewOrUpdated: true,
			existingConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				// Create initial config with static routes
				updates := []*sdcpb.Update{
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{Name: "ipv6"},
								{Name: "static-route", Key: map[string]string{
									"owner":       "owner-service1",
									"ipv6-prefix": "prefix-2001:db8::/32",
									"next-hop":    "nexthop-2001:db8::1",
								}},
								{Name: "bfd-enabled"},
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
					},
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{Name: "ipv6"},
								{Name: "static-route", Key: map[string]string{
									"owner":       "owner-service2",
									"ipv6-prefix": "prefix-2001:db8:1::/48",
									"next-hop":    "nexthop-2001:db8::2",
								}},
								{Name: "bfd-enabled"},
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}},
					},
				}
				return updates, nil
			},
			runningConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				// Same as existing config for this test
				updates := []*sdcpb.Update{
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{Name: "ipv6"},
								{Name: "static-route", Key: map[string]string{
									"owner":       "owner-service1",
									"ipv6-prefix": "prefix-2001:db8::/32",
									"next-hop":    "nexthop-2001:db8::1",
								}},
								{Name: "bfd-enabled"},
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
					},
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{Name: "ipv6"},
								{Name: "static-route", Key: map[string]string{
									"owner":       "owner-service2",
									"ipv6-prefix": "prefix-2001:db8:1::/48",
									"next-hop":    "nexthop-2001:db8::2",
								}},
								{Name: "bfd-enabled"},
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}},
					},
				}
				return updates, nil
			},
			newConfig: func(ctx context.Context, converter *utils.Converter) ([]*sdcpb.Update, error) {
				// Only keep one static route, delete the other
				updates := []*sdcpb.Update{
					{
						Path: &sdcpb.Path{
							Elem: []*sdcpb.PathElem{
								{Name: "ipv6"},
								{Name: "static-route", Key: map[string]string{
									"owner":       "owner-service2",
									"ipv6-prefix": "prefix-2001:db8:1::/48",
									"next-hop":    "nexthop-2001:db8::2",
								}},
								{Name: "bfd-enabled"},
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}},
					},
				}
				return updates, nil
			},
			expected: `<ipv6>
  <static-route operation="delete">
    <owner>owner-service1</owner>
    <ipv6-prefix>prefix-2001:db8::/32</ipv6-prefix>
    <next-hop>nexthop-2001:db8::1</next-hop>
  </static-route>
</ipv6>
`,
			honorNamespace:         false,
			operationWithNamespace: false,
			useOperationRemove:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.skip {
				t.Skip("Need to reimplement these tests")
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// mock schema client
			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}
			owner := "owner1"

			ctx := context.Background()

			converter := utils.NewConverter(scb)

			tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			if tt.existingConfig != nil {
				existingUpds, err := tt.existingConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}
				err = testhelper.AddToRoot(ctx, root.Entry, existingUpds, testhelper.FlagsExisting, owner, 5)
				if err != nil {
					t.Fatal(err)
				}
			}
			fmt.Println("AFTER EXISTING:")
			fmt.Println(root.String())

			if tt.newConfig != nil {
				sharedTaskPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
				ownerDeleteMarker := processors.NewOwnerDeleteMarker(&processors.OwnerDeleteMarkerProcessorParams{Owner: owner, OnlyIntended: false})

				err = ownerDeleteMarker.Run(root.Entry, sharedTaskPool)
				if err != nil {
					t.Error(err)
					return
				}

				newUpds, err := tt.newConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}
				err = testhelper.AddToRoot(ctx, root.Entry, newUpds, testhelper.FlagsNew, owner, 5)
				if err != nil {
					t.Fatal(err)
				}
			}
			fmt.Println("AFTER NEW:")
			fmt.Println(root.String())
			if tt.runningConfig != nil {
				runningUpds, err := tt.runningConfig(ctx, converter)
				if err != nil {
					t.Error(err)
				}
				err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
				if err != nil {
					t.Fatal(err)
				}
			}
			fmt.Println("AFTER RUNNING:")

			t.Log(root.String())
			fmt.Println(root.String())

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{
				RenderOpts:             ops.RenderOpts{OnlyNewOrUpdated: tt.onlyNewOrUpdated},
				HonorNamespace:         tt.honorNamespace,
				OperationWithNamespace: tt.operationWithNamespace,
				UseOperationRemove:     tt.useOperationRemove,
			})
			if err != nil {
				t.Fatal(err)
			}

			xmlDoc.Indent(2)
			xmlDocStr, err := xmlDoc.WriteToString()
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(string(xmlDocStr))

			if diff := cmp.Diff(tt.expected, string(xmlDocStr)); diff != "" {
				t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
			}
		})
	}
}

func TestToXML_KeyLeafDeleteUpgradesToListEntryDelete(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
			"default": {
				AdminState:  sdcio_schema.SdcioModelNi_AdminState_enable,
				Description: ygot.String("Default NI"),
				Type:        sdcio_schema.SdcioModelNi_NiType_default,
				Name:        ygot.String("default"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	nameEntry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "network-instance", Key: map[string]string{"name": "default"}}, {Name: "name"}}})
	if err != nil {
		t.Fatal(err)
	}
	deleteLeaf := api.NewLeafEntry(
		types.NewUpdate(nameEntry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "default"}}, 5, "owner1", 0),
		testhelper.FlagsDelete,
		nameEntry,
	)
	nameEntry.GetLeafVariants().Add(deleteLeaf)
	err = nil
	if err != nil {
		t.Fatal(err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	want := `<network-instance operation="delete">
  <name>default</name>
</network-instance>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}

// TestToXML_ImporterPreservesMultiKeyOrder is a regression test for a bug where
// the config importer sorted a list's key leaves in place on the slice returned
// by GetContainer().GetKeys(). That slice is backed by the shared/cached schema,
// so sorting it alphabetically permanently reordered the schema's keys. Every
// later ToXML/ToJson then emitted list keys in alphabetical order instead of the
// YANG `key` definition order, which NETCONF requires (RFC 7950 §7.8.5).
func TestToXML_ImporterPreservesMultiKeyOrder(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	jsonConfAny := map[string]any{
		"sdcio_model:ipv6": map[string]any{
			"static-route": []any{
				map[string]any{
					"owner":       "owner-up",
					"ipv6-prefix": "prefix-default",
					"next-hop":    "nexthop-1",
					"bfd-enabled": true,
				},
			},
		},
	}

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	_, err = root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(jsonConfAny, "intent1", 500, false), testhelper.FlagsNew, vpf)
	if err != nil {
		t.Fatal(err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// Keys must appear in YANG definition order: owner, ipv6-prefix, next-hop.
	iOwner := strings.Index(xmlDocStr, "<owner>")
	iPrefix := strings.Index(xmlDocStr, "<ipv6-prefix>")
	iNextHop := strings.Index(xmlDocStr, "<next-hop>")
	if iOwner == -1 || iPrefix == -1 || iNextHop == -1 {
		t.Fatalf("expected all three key elements in output, got:\n%s", xmlDocStr)
	}
	if iOwner >= iPrefix || iPrefix >= iNextHop {
		t.Fatalf("list keys not in YANG definition order (want owner < ipv6-prefix < next-hop), got:\n%s", xmlDocStr)
	}
}

func TestToXML_DoubleKeyLeafDeleteUpgradesToListEntryDelete(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
			{Key2: "k2foo", Key1: "k1bar"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("somevalue"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	// Navigate to the key2 leaf and mark it for deletion — this should promote to a list-entry delete
	key2Entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "doublekey", Key: map[string]string{"key2": "k2foo", "key1": "k1bar"}},
		{Name: "key2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	deleteLeaf := api.NewLeafEntry(
		types.NewUpdate(key2Entry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "k2foo"}}, 5, "owner1", 0),
		testhelper.FlagsDelete,
		key2Entry,
	)
	key2Entry.GetLeafVariants().Add(deleteLeaf)

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// YANG schema declares `key "key2 key1"`, so key elements must follow that
	// schema order (key2 first), not alphabetical order.
	want := `<doublekey operation="delete">
  <key2>k2foo</key2>
  <key1>k1bar</key1>
</doublekey>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}

// TestToXML_DoubleKeyLeafDeleteOnlyDeletesTargetEntry verifies that when multiple
// list entries share the same first key (key1), deleting a key leaf on one entry
// only generates a delete for that specific entry — not for siblings.
func TestToXML_DoubleKeyLeafDeleteOnlyDeletesTargetEntry(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	// Three entries: two share key1=k1bar (different key2), one has key1=k1other.
	// We will delete the key2 leaf of [k1bar][k2foo] only.
	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
			{Key2: "k2foo", Key1: "k1bar"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val1"),
			},
			{Key2: "k2other", Key1: "k1bar"}: {
				Key2:    ygot.String("k2other"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val2"),
			},
			{Key2: "k2foo", Key1: "k1other"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1other"),
				Mandato: ygot.String("val3"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the key2 leaf of [k1bar][k2foo] only — should promote to a list-entry delete for that entry.
	key2Entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "doublekey", Key: map[string]string{"key1": "k1bar", "key2": "k2foo"}},
		{Name: "key2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	deleteLeaf := api.NewLeafEntry(
		types.NewUpdate(key2Entry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "k2foo"}}, 5, "owner1", 0),
		testhelper.FlagsDelete,
		key2Entry,
	)
	key2Entry.GetLeafVariants().Add(deleteLeaf)

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// Only [k1bar][k2foo] should be deleted; [k1bar][k2other] and [k1other][k2foo] must not appear.
	// YANG schema declares `key "key2 key1"`, so key elements must follow that
	// schema order (key2 first), not alphabetical order.
	want := `<doublekey operation="delete">
  <key2>k2foo</key2>
  <key1>k1bar</key1>
</doublekey>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}

// TestToXML_DoubleKeyLeafDeleteViaKey1 verifies that deleting the key1 leaf (rather
// than key2) also promotes to a list-entry delete for the correct entry.
func TestToXML_DoubleKeyLeafDeleteViaKey1(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
			{Key2: "k2foo", Key1: "k1bar"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val1"),
			},
			{Key2: "k2other", Key1: "k1bar"}: {
				Key2:    ygot.String("k2other"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val2"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	// Delete via key1 leaf of [k1bar][k2foo].
	key1Entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "doublekey", Key: map[string]string{"key1": "k1bar", "key2": "k2foo"}},
		{Name: "key1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	key1Entry.GetLeafVariants().Add(api.NewLeafEntry(
		types.NewUpdate(key1Entry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "k1bar"}}, 5, "owner1", 0),
		testhelper.FlagsDelete,
		key1Entry,
	))

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// Only [k1bar][k2foo] deleted; [k1bar][k2other] must not appear.
	// YANG schema declares `key "key2 key1"`, so key elements must follow that
	// schema order (key2 first), not alphabetical order.
	want := `<doublekey operation="delete">
  <key2>k2foo</key2>
  <key1>k1bar</key1>
</doublekey>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}

// TestToXML_DoubleKeyNonKeyLeafDeleteDoesNotPromote verifies that deleting a
// non-key leaf (mandato) emits a targeted leaf delete, NOT a list-entry delete.
func TestToXML_DoubleKeyNonKeyLeafDeleteDoesNotPromote(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
			{Key2: "k2foo", Key1: "k1bar"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val1"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the non-key leaf "mandato".
	mandatoEntry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "doublekey", Key: map[string]string{"key1": "k1bar", "key2": "k2foo"}},
		{Name: "mandato"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	mandatoEntry.GetLeafVariants().Add(api.NewLeafEntry(
		types.NewUpdate(mandatoEntry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "val1"}}, 5, "owner1", 0),
		testhelper.FlagsDelete,
		mandatoEntry,
	))

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// The list entry itself must NOT carry operation="delete";
	// only the mandato child should carry it.
	// YANG schema declares `key "key2 key1"`, so key elements must be in schema order.
	want := `<doublekey>
  <key2>k2foo</key2>
  <key1>k1bar</key1>
  <mandato operation="delete"/>
</doublekey>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}

// TestToXML_SensitiveRedaction verifies that ToXML replaces sensitive-leaf values
// with the redaction sentinel when IncludeSensitive=false, and returns the real
// value when IncludeSensitive=true.
func TestToXML_SensitiveRedaction(t *testing.T) {
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

			xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{
				RenderOpts: ops.RenderOpts{IncludeSensitive: tt.exposeSensitive},
			})
			if err != nil {
				t.Fatal(err)
			}
			utils.XmlRecursiveSortElementsByTagName(&xmlDoc.Element)
			xmlDoc.Indent(2)
			xmlDocStr, err := xmlDoc.WriteToString()
			if err != nil {
				t.Fatal(err)
			}

			pattElem := xmlDoc.FindElement("//patterntest")
			if pattElem == nil {
				t.Fatalf("patterntest element not found in XML output:\n%s", xmlDocStr)
			}
			if diff := cmp.Diff(tt.wantPatterntest, pattElem.Text()); diff != "" {
				t.Errorf("patterntest value mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestToXML_DoubleKeyTwoSimultaneousListEntryDeletes verifies that deleting a key
// leaf on two different list entries at the same time produces two independent
// operation="delete" entries in the output.
func TestToXML_DoubleKeyTwoSimultaneousListEntryDeletes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	converter := utils.NewConverter(scb)
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	runningCfg := &sdcio_schema.Device{
		Patterntest: ygot.String("hallo 00"),
		Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
			{Key2: "k2foo", Key1: "k1bar"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val1"),
			},
			{Key2: "k2other", Key1: "k1bar"}: {
				Key2:    ygot.String("k2other"),
				Key1:    ygot.String("k1bar"),
				Mandato: ygot.String("val2"),
			},
			{Key2: "k2foo", Key1: "k1other"}: {
				Key2:    ygot.String("k2foo"),
				Key1:    ygot.String("k1other"),
				Mandato: ygot.String("val3"),
			},
		},
	}
	runningUpds, err := testhelper.ExpandUpdateFromConfig(ctx, runningCfg, converter)
	if err != nil {
		t.Fatal(err)
	}
	err = testhelper.AddToRoot(ctx, root.Entry, runningUpds, testhelper.FlagsExisting, consts.RunningIntentName, consts.RunningValuesPrio)
	if err != nil {
		t.Fatal(err)
	}

	// Delete key2 leaf on [k1bar][k2foo] and [k1other][k2foo] simultaneously.
	for _, keys := range []map[string]string{
		{"key1": "k1bar", "key2": "k2foo"},
		{"key1": "k1other", "key2": "k2foo"},
	} {
		entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, &sdcpb.Path{Elem: []*sdcpb.PathElem{
			{Name: "doublekey", Key: keys},
			{Name: "key2"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		entry.GetLeafVariants().Add(api.NewLeafEntry(
			types.NewUpdate(entry, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: keys["key2"]}}, 5, "owner1", 0),
			testhelper.FlagsDelete,
			entry,
		))
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	xmlDoc, err := ops.ToXML(ctx, root.Entry, ops.XMLRenderOpts{RenderOpts: ops.RenderOpts{OnlyNewOrUpdated: true}})
	if err != nil {
		t.Fatal(err)
	}
	xmlDoc.Indent(2)
	xmlDocStr, err := xmlDoc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}

	// Both targeted entries deleted; [k1bar][k2other] must not appear.
	// YANG schema declares `key "key2 key1"`, so key elements must be in schema order.
	want := `<doublekey operation="delete">
  <key2>k2foo</key2>
  <key1>k1bar</key1>
</doublekey>
<doublekey operation="delete">
  <key2>k2foo</key2>
  <key1>k1other</key1>
</doublekey>
`
	if diff := cmp.Diff(want, xmlDocStr); diff != "" {
		t.Fatalf("ToXML() mismatch (-want +got)\n%s", diff)
	}
}
