package tree

import (
	"context"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/consts"
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

				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				_, err = loadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				err = root.FinishInsertionPhase(ctx)
				if err != nil {
					t.Fatal(err)
				}

				return root
			},
			want: []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "value":{"stringVal":"foocaseval"}, "owner":"owner1"}]}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Foo"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"ethernet-1/1"}, "owner":"owner1"}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"description", "value":{"stringVal":"Subinterface 0"}, "owner":"owner1"}, {"name":"index", "value":{"uintVal":"0"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}, "owner":"owner1"}]}]}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"owner1"}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "value":{"stringVal":"disable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Default NI"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"default"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}, "owner":"owner1"}]}]}, {"name":"patterntest", "value":{"stringVal":"hallo 00"}, "owner":"owner1"}]}`),
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

				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				_, err = loadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, false, flagsNew)
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
			want:            []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "value":{"stringVal":"foocaseval"}, "owner":"owner1"}]}, {"name":"log", "value":{"boolVal":false}, "owner":"default"}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Foo"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"ethernet-1/1"}, "owner":"owner1"}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"admin-state", "value":{"stringVal":"enable"}, "owner":"default"}, {"name":"description", "value":{"stringVal":"Subinterface 0"}, "owner":"owner1"}, {"name":"index", "value":{"uintVal":"0"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}, "owner":"owner1"}]}]}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"owner1"}, {"name":"with-default", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}, "owner":"default"}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "value":{"stringVal":"disable"}, "owner":"owner1"}, {"name":"description", "value":{"stringVal":"Default NI"}, "owner":"owner1"}, {"name":"name", "value":{"stringVal":"default"}, "owner":"owner1"}, {"name":"type", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}, "owner":"owner1"}]}]}, {"name":"patterntest", "value":{"stringVal":"hallo 00"}, "owner":"owner1"}]}`),
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

				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatal(err)
				}

				conf1 := config1()
				_, err = loadYgotStructIntoTreeRoot(ctx, conf1, root, owner1, 5, false, flagsNew)
				if err != nil {
					t.Fatal(err)
				}

				conf2 := config2()
				_, err = loadYgotStructIntoTreeRoot(ctx, conf2, root, owner2, 10, false, flagsNew)
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

				_, err = loadYgotStructIntoTreeRoot(ctx, running, root, consts.RunningIntentName, consts.RunningValuesPrio, false, flagsExisting)
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
			want:            []byte(`{"name":"root", "childs":[{"name":"choices", "childs":[{"name":"case1", "childs":[{"name":"case-elem", "childs":[{"name":"elem", "owner":"owner1", "value":{"stringVal":"foocaseval"}}]}, {"name":"log", "owner":"default", "value":{"boolVal":false}}]}]}, {"name":"interface", "childs":[{"name":"ethernet-1/1", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Foo"}, "deviation_value":{"stringVal":"Changed Description"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"ethernet-1/1"}}, {"name":"subinterface", "childs":[{"name":"0", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Subinterface 0"}}, {"name":"index", "owner":"owner1", "value":{"uintVal":"0"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}}]}]}]}, {"name":"ethernet-1/2", "childs":[{"name":"admin-state", "owner":"owner2", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Foo"}}, {"name":"name", "owner":"owner2", "value":{"stringVal":"ethernet-1/2"}}, {"name":"subinterface", "childs":[{"name":"5", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Subinterface 5"}}, {"name":"index", "owner":"owner2", "value":{"uintVal":"5"}}, {"name":"type", "owner":"owner2", "value":{"identityrefVal":{"value":"routed", "prefix":"sdcio_model_common", "module":"sdcio_model_common"}}}]}]}]}, {"name":"ethernet-1/3", "childs":[{"name":"admin-state", "owner":"default", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"running", "value":{"stringVal":"ethernet-1/3 description"}}, {"name":"name", "owner":"running", "value":{"stringVal":"ethernet-1/3"}}]}]}, {"name":"leaflist", "childs":[{"name":"entry", "owner":"owner1", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}}, {"name":"with-default", "owner":"default", "value":{"leaflistVal":{"element":[{"stringVal":"foo"}, {"stringVal":"bar"}]}}}]}, {"name":"network-instance", "childs":[{"name":"default", "childs":[{"name":"admin-state", "owner":"owner1", "value":{"stringVal":"disable"}}, {"name":"description", "owner":"owner1", "value":{"stringVal":"Default NI"}}, {"name":"name", "owner":"owner1", "value":{"stringVal":"default"}}, {"name":"type", "owner":"owner1", "value":{"identityrefVal":{"value":"default", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}}]}, {"name":"other", "childs":[{"name":"admin-state", "owner":"owner2", "value":{"stringVal":"enable"}}, {"name":"description", "owner":"owner2", "value":{"stringVal":"Other NI"}}, {"name":"name", "owner":"owner2", "value":{"stringVal":"other"}}, {"name":"type", "owner":"owner2", "value":{"identityrefVal":{"value":"ip-vrf", "prefix":"sdcio_model_ni", "module":"sdcio_model_ni"}}}]}]}, {"name":"patterntest", "owner":"owner1", "value":{"stringVal":"hallo 00"}, "deviation_value":{"stringVal":"hallo 0"}}]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			treeRoot := tt.r(t)

			sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
			vPool := sharedPool.NewVirtualPool(pool.VirtualFailFast)

			bp := NewBlameConfigProcessor(NewBlameConfigProcessorConfig(tt.includeDefaults))
			got, err := bp.Run(ctx, treeRoot.Entry, vPool)
			if err != nil {
				t.Errorf("BlameConfig() error %s", err)
				return
			}

			t.Log(got.ToString())

			want := &sdcpb.BlameTreeElement{}
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
