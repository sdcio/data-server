package processors_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_sharedEntryAttributes_BlameConfig(t *testing.T) {
	owner1 := "owner1"
	owner2 := "owner2"
	ctx := context.TODO()

	tests := []struct {
		name            string
		r               func(t *testing.T) *tree.RootEntry
		includeDefaults bool
		want            []byte
		wantErr         bool
	}{
		{
			name: "without defaults",
			r: func(t *testing.T) *tree.RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := testhelper.Config1()
				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root.Entry, owner1, 5, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "owner":"owner1", "value":{"stringVal":"foocaseval"}}]}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "keyName":"name", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Foo"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"ethernet-1/1"}}, {"name":"subinterface", "childs":[{"name":"0", "keyName":"index", "childs":[{"name":"description", "owner":"owner1", "value":{"stringVal":"Subinterface 0"}}, {"name":"index", "owner":"owner1", "value":{"uintVal":"0"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}}]}]}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "owner":"owner1", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}}]}, {"name":"network-instance", "childs":[{"name":"default", "keyName":"name", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"disable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Default NI"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"default"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}}]}]}, {"name":"patterntest", "owner":"owner1", "value":{"stringVal":"hallo 00"}}]}`),
		},
		{
			name: "with defaults",
			r: func(t *testing.T) *tree.RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := testhelper.Config1()
				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root.Entry, owner1, 5, false, flagsNew)
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
			want:            []byte(`{"name":"root","childs":[{"name":"choices","childs":[{"name":"case1","childs":[{"name":"case-elem","childs":[{"name":"elem","owner":"owner1","value":{"stringVal":"foocaseval"}}]},{"name":"log","owner":"default","value":{"boolVal":false}}]}]},{"name":"interface","childs":[{"name":"ethernet-1/1","keyName":"name","childs":[{"name":"admin-state","owner":"owner1","value":{"stringVal":"enable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Foo"}},{"name":"name","owner":"owner1","value":{"stringVal":"ethernet-1/1"}},{"name":"subinterface","childs":[{"name":"0","keyName":"index","childs":[{"name":"admin-state","owner":"default","value":{"stringVal":"enable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Subinterface 0"}},{"name":"index","owner":"owner1","value":{"uintVal":"0"}},{"name":"type","owner":"owner1","value":{"identityrefVal":{"value":"routed","prefix":"sdcio_model_common","module":"sdcio_model_common"}}}]}]}]}]},{"name":"leaflist","childs":[{"name":"entry","owner":"owner1","value":{"leaflistVal":{"element":[{"stringVal":"foo"},{"stringVal":"bar"}]}}},{"name":"with-default","owner":"default","value":{"leaflistVal":{"element":[{"stringVal":"foo"},{"stringVal":"bar"}]}}}]},{"name":"network-instance","childs":[{"name":"default","keyName":"name","childs":[{"name":"admin-state","owner":"owner1","value":{"stringVal":"disable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Default NI"}},{"name":"name","owner":"owner1","value":{"stringVal":"default"}},{"name":"type","owner":"owner1","value":{"identityrefVal":{"value":"default","prefix":"sdcio_model_ni","module":"sdcio_model_ni"}}}]}]},{"name":"patterntest","owner":"owner1","value":{"stringVal":"hallo 00"}}]}`),
		},
		{
			name: "with defaults multiple intents",
			r: func(t *testing.T) *tree.RootEntry {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()

				scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
				if err != nil {
					t.Fatal(err)
				}

				tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := testhelper.Config1()
				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf1, root.Entry, owner1, 5, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				conf2 := testhelper.Config2()
				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, conf2, root.Entry, owner2, 10, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				running := testhelper.Config1()

				running.Interface["ethernet-1/1"].Description = ygot.String("Changed Description")
				running.Interface["ethernet-1/3"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/3"),
					Description: ygot.String("ethernet-1/3 description"),
				}

				running.Patterntest = ygot.String("hallo 0")

				_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, running, root.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false, flagsExisting)
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
			want:            []byte(`{"name":"root","childs":[{"name":"choices","childs":[{"name":"case1","childs":[{"name":"case-elem","childs":[{"name":"elem","owner":"owner1","value":{"stringVal":"foocaseval"}}]},{"name":"log","owner":"default","value":{"boolVal":false}}]}]},{"name":"interface","childs":[{"name":"ethernet-1/1","keyName":"name","childs":[{"name":"admin-state","owner":"owner1","value":{"stringVal":"enable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Foo"},"deviationValue":{"stringVal":"Changed Description"}},{"name":"name","owner":"owner1","value":{"stringVal":"ethernet-1/1"}},{"name":"subinterface","childs":[{"name":"0","keyName":"index","childs":[{"name":"admin-state","owner":"default","value":{"stringVal":"enable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Subinterface 0"}},{"name":"index","owner":"owner1","value":{"uintVal":"0"}},{"name":"type","owner":"owner1","value":{"identityrefVal":{"value":"routed","prefix":"sdcio_model_common","module":"sdcio_model_common"}}}]}]}]},{"name":"ethernet-1/2","keyName":"name","childs":[{"name":"admin-state","owner":"owner2","value":{"stringVal":"enable"}},{"name":"description","owner":"owner2","value":{"stringVal":"Foo"}},{"name":"name","owner":"owner2","value":{"stringVal":"ethernet-1/2"}},{"name":"subinterface","childs":[{"name":"5","keyName":"index","childs":[{"name":"admin-state","owner":"default","value":{"stringVal":"enable"}},{"name":"description","owner":"owner2","value":{"stringVal":"Subinterface 5"}},{"name":"index","owner":"owner2","value":{"uintVal":"5"}},{"name":"type","owner":"owner2","value":{"identityrefVal":{"value":"routed","prefix":"sdcio_model_common","module":"sdcio_model_common"}}}]}]}]},{"name":"ethernet-1/3","keyName":"name","childs":[{"name":"admin-state","owner":"default","value":{"stringVal":"enable"}},{"name":"description","owner":"running","value":{"stringVal":"ethernet-1/3 description"}},{"name":"name","owner":"running","value":{"stringVal":"ethernet-1/3"}}]}]},{"name":"leaflist","childs":[{"name":"entry","owner":"owner1","value":{"leaflistVal":{"element":[{"stringVal":"foo"},{"stringVal":"bar"}]}}},{"name":"with-default","owner":"default","value":{"leaflistVal":{"element":[{"stringVal":"foo"},{"stringVal":"bar"}]}}}]},{"name":"network-instance","childs":[{"name":"default","keyName":"name","childs":[{"name":"admin-state","owner":"owner1","value":{"stringVal":"disable"}},{"name":"description","owner":"owner1","value":{"stringVal":"Default NI"}},{"name":"name","owner":"owner1","value":{"stringVal":"default"}},{"name":"type","owner":"owner1","value":{"identityrefVal":{"value":"default","prefix":"sdcio_model_ni","module":"sdcio_model_ni"}}}]},{"name":"other","keyName":"name","childs":[{"name":"admin-state","owner":"owner2","value":{"stringVal":"enable"}},{"name":"description","owner":"owner2","value":{"stringVal":"Other NI"}},{"name":"name","owner":"owner2","value":{"stringVal":"other"}},{"name":"type","owner":"owner2","value":{"identityrefVal":{"value":"ip-vrf","prefix":"sdcio_model_ni","module":"sdcio_model_ni"}}}]}]},{"name":"patterntest","owner":"owner1","value":{"stringVal":"hallo 00"},"deviationValue":{"stringVal":"hallo 0"}}]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			treeRoot := tt.r(t)

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			bp := processors.NewBlameConfigProcessor(&processors.BlameConfigProcessorParams{IncludeDefaults: tt.includeDefaults})
			got, err := bp.Run(ctx, treeRoot.Entry, sharedPool)
			if err != nil {
				t.Errorf("BlameConfig() error %s", err)
				return
			}

			t.Log(got.ToString())
			t.Log(got.StringXPath())

			want := &sdcpb.BlameTreeElement{}
			err = protojson.Unmarshal([]byte(tt.want), want)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON to proto: %v", err)
			}
			// gotB, err := protojson.Marshal(got)
			// if err != nil {
			// 	t.Fatalf("failed to marshal proto to JSON: %v", err)
			// }
			// fmt.Println(string(gotB))

			if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Errorf("BlameConfig() mismatch (-want +got)\n%s", diff)
				return
			}
		})
	}
}
