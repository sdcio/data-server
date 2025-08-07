package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	schema_server "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_sharedEntryAttributes_checkAndCreateKeysAsLeafs(t *testing.T) {
	controller := gomock.NewController(t)
	defer controller.Finish()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	tc := NewTreeContext(scb, "intent1")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	flags := types.NewUpdateInsertFlags()
	flags.SetNewFlag()

	prio := int32(5)
	intentName := "intent1"

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "description"}, testhelper.GetStringTvProto("MyDescription"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate([]string{"doublekey", "k1.1", "k1.3", "mandato"}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	t.Log(root.String())

	fmt.Println(root.String())
	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(root.String())

	// TODO: check the result
}

func Test_sharedEntryAttributes_DeepCopy(t *testing.T) {
	owner1 := "owner1"
	tests := []struct {
		name string
		root func() *RootEntry
	}{
		{
			name: "just rootEntry",
			root: func() *RootEntry {
				tc := NewTreeContext(nil, owner1)
				r := &RootEntry{
					sharedEntryAttributes: &sharedEntryAttributes{
						pathElemName:     "__root__",
						childs:           newChildMap(),
						childsMutex:      sync.RWMutex{},
						choicesResolvers: choiceResolvers{},
						parent:           nil,
						treeContext:      tc,
					},
				}
				r.leafVariants = newLeafVariants(tc, r.sharedEntryAttributes)
				return r
			},
		},
		{
			name: "more complex tree",
			root: func() *RootEntry {
				// create a gomock controller
				controller := gomock.NewController(t)
				defer controller.Finish()

				ctx := context.Background()

				sc, schema, err := testhelper.InitSDCIOSchema()
				if err != nil {
					t.Fatal(err)
				}
				scb := schemaClient.NewSchemaClientBound(schema, sc)
				tc := NewTreeContext(scb, owner1)

				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Error(err)
				}

				jconfStr, err := ygot.EmitJSON(config1(), &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: true,
				})
				if err != nil {
					t.Error(err)
				}

				var jsonConfAny any
				err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
				if err != nil {
					t.Error(err)
				}

				newFlag := types.NewUpdateInsertFlags()

				err = root.ImportConfig(ctx, types.PathSlice{}, jsonImporter.NewJsonTreeImporter(jsonConfAny), owner1, 500, newFlag)
				if err != nil {
					t.Error(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Error(err)
				}
				return root
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.root()

			ctx := context.Background()

			newRoot, err := root.DeepCopy(ctx)
			if err != nil {
				return
			}

			if diff := cmp.Diff(root.String(), newRoot.String()); diff != "" {
				t.Errorf("mismatching trees (-want +got)\n%s", diff)
				return
			}
		})
	}
}

func Test_sharedEntryAttributes_DeleteSubtree(t *testing.T) {
	owner1 := "owner1"
	owner2 := "owner2"
	ctx := context.TODO()
	type args struct {
		relativePath types.PathSlice
		owner        string
	}
	tests := []struct {
		name                  string
		sharedEntryAttributes func(t *testing.T) *sharedEntryAttributes
		args                  args
		want                  bool
		wantErr               bool
	}{
		{
			name: "one",
			sharedEntryAttributes: func(t *testing.T) *sharedEntryAttributes {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config2(), root, owner2, 10, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root.sharedEntryAttributes
			},
			args: args{
				relativePath: types.PathSlice{"interface"},
				owner:        owner1,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "wrong path",
			sharedEntryAttributes: func(t *testing.T) *sharedEntryAttributes {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config1(), root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, config2(), root, owner2, 10, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root.sharedEntryAttributes
			},
			args: args{
				relativePath: types.PathSlice{"interface", "ethernet-1/27"},
				owner:        owner1,
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.sharedEntryAttributes(t)
			got, err := s.DeleteSubtree(tt.args.relativePath, tt.args.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("sharedEntryAttributes.DeleteSubtree() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("sharedEntryAttributes.DeleteSubtree() = %v, want %v", got, tt.want)
			}
			e, err := s.Navigate(ctx, tt.args.relativePath, false, false)
			if err != nil {
				t.Error(err)
				return
			}
			les := []*LeafEntry{}
			result := e.GetByOwner(tt.args.owner, les)
			if len(result) > 0 {
				t.Errorf("expected all elements under %s to be deleted for owner %s but got %d elements", strings.Join(tt.args.relativePath, "/"), tt.args.owner, len(result))
				return
			}
		})
	}
}

func Test_sharedEntryAttributes_GetListChilds(t *testing.T) {
	owner1 := "owner1"
	ctx := context.TODO()
	device := func(t *testing.T) *RootEntry {
		d := &sdcio_schema.Device{
			Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
				{
					Key1: "k1.1",
					Key2: "k1.2",
				}: {
					Key1:    ygot.String("k1.1"),
					Key2:    ygot.String("k1.2"),
					Mandato: ygot.String("TheMandatoryValueOther"),
					Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
						Value1: ygot.String("containerval1.1"),
						Value2: ygot.String("containerval1.2"),
					},
				},
				{
					Key1: "k2.1",
					Key2: "k2.2",
				}: {
					Key1:    ygot.String("k2.1"),
					Key2:    ygot.String("k2.2"),
					Mandato: ygot.String("TheMandatoryValue2"),
					Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
						Value1: ygot.String("containerval2.1"),
						Value2: ygot.String("containerval2.2"),
					},
				},
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
		if err != nil {
			t.Fatal(err)
		}

		tc := NewTreeContext(scb, owner1)
		root, err := NewTreeRoot(ctx, tc)
		if err != nil {
			t.Fatal(err)
		}

		err = testhelper.LoadYgotStructIntoTreeRoot(ctx, d, root, owner1, 5, flagsNew)
		if err != nil {
			t.Fatal(err)
		}
		return root
	}

	tests := []struct {
		name      string
		path      []string
		wantKeys  []string
		wantNames []string
		wantErr   bool
	}{
		{
			name:      "Double Key - pass",
			wantNames: []string{"k2.2", "k1.2"},
			wantKeys:  []string{"key1", "key2", "cont", "mandato"},
			path:      []string{"doublekey"},
		},
		{
			name:    "nil schema",
			path:    []string{"doublekey", "k1.1"},
			wantErr: true,
		},
		{
			name:    "non container",
			path:    []string{"doublekey", "k1.1", "k1.2", "mandato"},
			wantErr: true,
		},
		{
			name:    "container not a list",
			path:    []string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := device(t).Navigate(ctx, tt.path, true, false)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := e.GetListChilds()
			if (err != nil) != tt.wantErr {
				t.Errorf("sharedEntryAttributes.GetListChilds() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			elemNames := []string{}
			elemChilds := map[string][]string{}
			for _, elem := range got {
				elemNames = append(elemNames, elem.PathName())
				elemChilds[elem.PathName()] = []string{}
				for k := range elem.getChildren() {
					elemChilds[elem.PathName()] = append(elemChilds[elem.PathName()], k)
				}
			}
			slices.Sort(elemNames)
			slices.Sort(tt.wantNames)

			if diff := cmp.Diff(tt.wantNames, elemNames); diff != "" {
				t.Errorf("mismatch (-want +got)\n%s", diff)
				return
			}

			slices.Sort(tt.wantKeys)
			for k, v := range elemChilds {
				slices.Sort(v)
				if diff := cmp.Diff(tt.wantKeys, v); diff != "" {
					t.Errorf("key %s mismatch (-want +got)\n%s", k, diff)
					return
				}
			}

		})
	}
}

func Test_sharedEntryAttributes_GetDeviations(t *testing.T) {
	owner1 := "owner1"
	ctx := context.TODO()

	tests := []struct {
		name string
		s    func(t *testing.T) *RootEntry
		want []*types.DeviationEntry
	}{
		{
			name: "one",
			s: func(t *testing.T) *RootEntry {

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				running := config1()

				running.Interface["ethernet-1/1"].Description = ygot.String("Changed Description")
				running.Interface["ethernet-1/3"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/3"),
					Description: ygot.String("ethernet-1/3 description"),
				}

				running.Patterntest = ygot.String("hallo 0")

				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, running, root, RunningIntentName, RunningValuesPrio, flagsExisting)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []*types.DeviationEntry{
				// one
				types.NewDeviationEntry(
					owner1,
					types.DeviationReasonNotApplied,
					&schema_server.Path{
						Elem: []*schema_server.PathElem{
							{Name: "interface", Key: map[string]string{"name": "ethernet-1/1"}},
							{Name: "description"}},
					},
				).SetCurrentValue(testhelper.GetStringTvProto("Changed Description")).SetExpectedValue(testhelper.GetStringTvProto("Foo")),
				// two
				types.NewDeviationEntry(
					owner1,
					types.DeviationReasonNotApplied,
					&schema_server.Path{
						Elem: []*schema_server.PathElem{
							{Name: "patterntest"}},
					},
				).SetCurrentValue(testhelper.GetStringTvProto("hallo 0")).SetExpectedValue(testhelper.GetStringTvProto("foo")),
				// three
				types.NewDeviationEntry(
					RunningIntentName,
					types.DeviationReasonUnhandled,
					&schema_server.Path{
						Elem: []*schema_server.PathElem{
							{Name: "interface", Key: map[string]string{"name": "ethernet-1/3"}},
							{Name: "description"}},
					},
				).SetCurrentValue(testhelper.GetStringTvProto("ethernet-1/3 description")).SetExpectedValue(nil),
				// four
				types.NewDeviationEntry(
					RunningIntentName,
					types.DeviationReasonUnhandled,
					&schema_server.Path{
						Elem: []*schema_server.PathElem{
							{Name: "interface", Key: map[string]string{"name": "ethernet-1/3"}},
							{Name: "name"}},
					},
				).SetCurrentValue(testhelper.GetStringTvProto("ethernet-1/3")).SetExpectedValue(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.s(t)
			ch := make(chan *types.DeviationEntry, 100)
			root.GetDeviations(ch)
			close(ch)

			result := []string{}
			for entry := range ch {
				result = append(result, entry.String())
			}

			expected := []string{}
			for _, entry := range tt.want {
				expected = append(expected, entry.String())
			}
			// sort slices
			slices.Sort(result)
			slices.Sort(expected)

			// combine into single string
			expectedString := strings.Join(expected, "\n")
			resultString := strings.Join(result, "\n")

			// diff the expected and result Strings
			if diff := cmp.Diff(expectedString, resultString); diff != "" {
				t.Errorf("mismatch (-want +got)\n%s", diff)
				return
			}

		})
	}
}

func Test_sharedEntryAttributes_getOrCreateChilds(t *testing.T) {
	ctx := context.TODO()
	owner1 := "owner1"

	tests := []struct {
		name        string
		path        types.PathSlice
		wantErr     bool
		errContains string
	}{
		{
			name: "one",
			path: types.PathSlice{"interface", "ethernet-1/1", "description"},
		},
		{
			name: "doublekey",
			path: types.PathSlice{"doublekey", "k1.1", "k1.2", "mandato"},
		},
		{
			name:        "non existing attribute",
			path:        types.PathSlice{"network-instance", "ni1", "protocol", "osgp"},
			wantErr:     true,
			errContains: "container protocol - unknown element osgp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}

			tc := NewTreeContext(scb, owner1)
			root, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}

			x, err := root.getOrCreateChilds(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("sharedEntryAttributes.getOrCreateChilds() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error: %s, got error %s", err.Error(), tt.errContains)
				}
				return
			}

			if x.Path().String() != tt.path.String() {
				t.Errorf("%s != %s", x.Path().String(), tt.path.String())
			}
		})
	}
}

func Test_sharedEntryAttributes_validateMandatory(t *testing.T) {
	ctx := context.TODO()
	owner1 := "owner1"

	tests := []struct {
		name string
		r    func(t *testing.T) *RootEntry
		want []*types.ValidationResultEntry
	}{
		{
			name: "no containers with mandatories",
			r: func(t *testing.T) *RootEntry {

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []*types.ValidationResultEntry{},
		},
		{
			name: "mandatories missing",
			r: func(t *testing.T) *RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := &sdcio_schema.Device{
					Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
						{
							Key1: "k1.1",
							Key2: "k1.2",
						}: {
							Key1: ygot.String("k1.1"),
							Key2: ygot.String("k1.2"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
					},
					NetworkInstance: map[string]*sdcio_schema.SdcioModel_NetworkInstance{
						"ni1": {
							Name:        ygot.String("ni1"),
							Description: ygot.String("ni1 Description"),
							Protocol: &sdcio_schema.SdcioModel_NetworkInstance_Protocol{
								Bgp: &sdcio_schema.SdcioModel_NetworkInstance_Protocol_Bgp{
									AdminState: sdcio_schema.SdcioModelNi_AdminState_enable,
								},
							},
						},
					},
				}
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []*types.ValidationResultEntry{
				types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child [mandato] does not exist, path: doublekey/k1.1/k1.2"), types.ValidationResultEntryTypeError),
				types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child [autonomous-system] does not exist, path: network-instance/ni1/protocol/bgp"), types.ValidationResultEntryTypeError),
				types.NewValidationResultEntry("unknown", fmt.Errorf("error mandatory child [router-id] does not exist, path: network-instance/ni1/protocol/bgp"), types.ValidationResultEntryTypeError),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.r(t)

			validationConfig := config.NewValidationConfig()
			validationConfig.SetDisableConcurrency(true)
			validationConfig.DisabledValidators.DisableAll()
			validationConfig.DisabledValidators.Mandatory = false

			validationResults, _ := root.Validate(ctx, validationConfig)

			results := []string{}
			for _, e := range validationResults {
				results = append(results, e.ErrorsString()...)
				results = append(results, e.WarningsString()...)
			}

			expected := types.ValidationResults{}
			for _, e := range tt.want {
				err := expected.AddEntry(e)
				if err != nil {
					t.Error(err)
					return
				}
			}

			expectedStrArr := []string{}
			for _, e := range expected {
				expectedStrArr = append(expectedStrArr, e.ErrorsString()...)
				expectedStrArr = append(expectedStrArr, e.WarningsString()...)
			}

			slices.Sort(results)
			slices.Sort(expectedStrArr)

			if diff := cmp.Diff(expectedStrArr, results); diff != "" {
				t.Errorf("mismatching validation messages (-want +got)\n%s", diff)
				return
			}

		})
	}
}

func Test_sharedEntryAttributes_BlameConfig(t *testing.T) {
	owner1 := "owner1"
	owner2 := "owner2"
	ctx := context.TODO()

	tests := []struct {
		name            string
		r               func(t *testing.T) *RootEntry
		includeDefaults bool
		want            []byte
		wantErr         bool
	}{
		{
			name: "without defaults",
			r: func(t *testing.T) *RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "value":{"stringVal":"foocaseval"}, "owner":"owner1"}]}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Foo"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"ethernet-1/1"}, "owner":"owner1"}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"description", "value":{"stringVal":"Subinterface 0"}, "owner":"owner1"}, {"name":"index", "value":{"uintVal":"0"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}, "owner":"owner1"}]}]}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"owner1"}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "value":{"stringVal":"disable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Default NI"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"default"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}, "owner":"owner1"}]}]}, {"name":"patterntest", "value":{"stringVal":"foo"}, "owner":"owner1"}]}`),
		},
		{
			name: "with defaults",
			r: func(t *testing.T) *RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			includeDefaults: true,
			want:            []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "value":{"stringVal":"foocaseval"}, "owner":"owner1"}]}, {"name":"log", "value":{"boolVal":false}, "owner":"default"}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Foo"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"ethernet-1/1"}, "owner":"owner1"}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"default"}, {"name":"description", "value":{"stringVal":"Subinterface 0"}, "owner":"owner1"}, {"name":"index", "value":{"uintVal":"0"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}, "owner":"owner1"}]}]}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"owner1"}, {"name":"with-default", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"default"}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "value":{"stringVal":"disable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Default NI"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"default"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}, "owner":"owner1"}]}]}, {"name":"patterntest", "value":{"stringVal":"foo"}, "owner":"owner1"}]}`),
		},
		{
			name: "with defaults multiple intents",
			r: func(t *testing.T) *RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := NewTreeContext(scb, owner1)
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				conf2 := config2()
				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf2, root, owner2, 10, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				running := config1()

				running.Interface["ethernet-1/1"].Description = ygot.String("Changed Description")
				running.Interface["ethernet-1/3"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/3"),
					Description: ygot.String("ethernet-1/3 description"),
				}

				running.Patterntest = ygot.String("hallo 0")

				err = testhelper.LoadYgotStructIntoTreeRoot(ctx, running, root, RunningIntentName, RunningValuesPrio, flagsExisting)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			includeDefaults: true,
			want:            []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "owner":"owner1", "value":{"stringVal":"foocaseval"}}]}, {"name":"log", "owner":"default", "value":{"boolVal":false}}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Foo"}, "deviation_value":{"stringVal":"Changed Description"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"ethernet-1/1"}}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Subinterface 0"}}, {"name":"index", "owner":"owner1", "value":{"uintVal":"0"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}}]}]}]}, {"name":"ethernet-1/2", "childs":[{"name":"admin-state", "owner":"owner2", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Foo"}}, {"name":"name", "owner":"owner2", "value":{"stringVal":"ethernet-1/2"}}, {"name":"subinterface", "childs":[{"name":"5", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Subinterface 5"}}, {"name":"index", "owner":"owner2", "value":{"uintVal":"5"}}, {"name":"type", "owner":"owner2", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}}]}]}]}, {"name":"ethernet-1/3", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"running", "value":{"stringVal":"ethernet-1/3 description"}}, {"name":"name", "owner":"running", "value":{"stringVal":"ethernet-1/3"}}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "owner":"owner1", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}}, {"name":"with-default", "owner":"default", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"disable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Default NI"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"default"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}}]}, {"name":"other", "childs":[{"name":"admin-state", "owner":"owner2", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Other NI"}}, {"name":"name", "owner":"owner2", "value":{"stringVal":"other"}}, {"name":"type", "owner":"owner2", "value":{"identityrefVal":{"value":"ip-vrf", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}}]}]}, {"name":"patterntest", "owner":"owner1", "value":{"stringVal":"foo"}, "deviation_value":{"stringVal":"hallo 0"}}]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.r(t)
			got, err := s.BlameConfig(tt.includeDefaults)
			if (err != nil) != tt.wantErr {
				t.Errorf("sharedEntryAttributes.BlameConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// // generate the want part via
			// // ---------------------------------------
			// gotJson, err := protojson.Marshal(got)
			// if err != nil {
			// 	t.Fatalf("failed to marshal proto to JSON: %v", err)
			// }
			// fmt.Println(string(gotJson))
			// return

			t.Log(got.ToString())

			want := &schema_server.BlameTreeElement{}
			err = protojson.Unmarshal([]byte(tt.want), want)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON to proto: %v", err)
			}

			if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Errorf("BlameConfig() mismatch (-want +got)\n%s", diff)
				return
			}
		})
	}
}

func Test_sharedEntryAttributes_ReApply(t *testing.T) {
	ctx := context.TODO()
	owner1 := "owner1"
	owner1Prio := int32(50)

	tests := []struct {
		name      string
		r         func(t *testing.T) *sdcio_schema.Device
		numDelete int
	}{
		{
			name: "multiple keys",
			r: func(t *testing.T) *sdcio_schema.Device {
				conf1 := &sdcio_schema.Device{
					Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
						{
							Key1: "k1.1",
							Key2: "k1.2",
						}: {
							Key1: ygot.String("k1.1"),
							Key2: ygot.String("k1.2"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
							Mandato: ygot.String("foo"),
						},
					},
				}
				return conf1
			},
			numDelete: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}

			tc := NewTreeContext(scb, owner1)
			root, err := NewTreeRoot(ctx, tc)
			if err != nil {
				t.Fatal(err)
			}
			updSlice := types.UpdateSlice{
				types.NewUpdate([]string{"doublekey", "k1.1", "k1.2", "mandato"}, testhelper.GetStringTvProto("TheMandatoryValue1"), owner1Prio, owner1, 0),
			}

			err = root.AddUpdatesRecursive(ctx, updSlice, flagsNew)
			if err != nil {
				t.Error(err)
			}

			fmt.Println(root.String())

			treepersist, err := root.TreeExport(owner1, owner1Prio)
			if err != nil {
				t.Error(err)
				return
			}

			persistByte, err := protojson.Marshal(treepersist)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Println("\nTreeExport:")
			fmt.Println(string(persistByte))

			tcNew := NewTreeContext(scb, owner1)
			newRoot, err := NewTreeRoot(ctx, tcNew)
			if err != nil {
				t.Fatal(err)
			}

			err = newRoot.ImportConfig(ctx, types.PathSlice{}, proto.NewProtoTreeImporter(treepersist.Root), owner1, owner1Prio, flagsExisting)
			if err != nil {
				t.Error(err)
				return
			}

			// mark owner delete
			newRoot.MarkOwnerDelete(owner1, false)

			err = newRoot.AddUpdatesRecursive(ctx, updSlice, flagsNew)
			if err != nil {
				t.Error(err)
			}

			fmt.Println(newRoot.String())

			err = newRoot.FinishInsertionPhase(ctx)
			if err != nil {
				t.Fatal(err)
			}

			deleteList, err := newRoot.GetDeletes(true)
			if err != nil {
				t.Error(err)
			}
			if len(deleteList) != tt.numDelete {
				t.Errorf("%d deltes expected, got %d\n%s", tt.numDelete, len(deleteList), strings.Join(deleteList.PathSlices().StringSlice(), ", "))
			}

		})
	}
}
