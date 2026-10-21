package tree

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
)

func BenchmarkAddUpdatesRecursive(b *testing.B) {
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		b.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)
	converter := utils.NewConverter(scb)
	ctx := context.Background()

	interfaceDevice := func(n int, start int) *sdcio_schema.Device {
		ifaces := make(map[string]*sdcio_schema.SdcioModel_Interface, n)
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("ethernet-1/%d", start+i)
			ifaces[name] = &sdcio_schema.SdcioModel_Interface{
				AdminState:    sdcio_schema.SdcioModelIf_AdminState_enable,
				Description:   ygot.String("Foo"),
				Name:          ygot.String(name),
				InterfaceType: ygot.String("ethernet"),
				Mtu:           ygot.Uint16(1500),
				Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
					0: {
						Description: ygot.String("Subinterface 0"),
						Type:        sdcio_schema.SdcioModelCommon_SiType_routed,
						Index:       ygot.Uint32(0),
						AdminState:  sdcio_schema.SdcioModelIf_AdminState_enable,
					},
				},
			}
		}
		return &sdcio_schema.Device{Interface: ifaces}
	}

	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tc := NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
				root, err := NewTreeRoot(ctx, tc)
				if err != nil {
					b.Fatal(err)
				}
				baseUpds, err := testhelper.ExpandUpdateFromConfig(ctx, interfaceDevice(n, 1), converter)
				if err != nil {
					b.Fatal(err)
				}
				if err = testhelper.AddToRoot(ctx, root.Entry, baseUpds, types.NewUpdateInsertFlags(), "bench", 5); err != nil {
					b.Fatal(err)
				}
				extraUpds, err := testhelper.ExpandUpdateFromConfig(ctx, interfaceDevice(1, n+1), converter)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err = testhelper.AddToRoot(ctx, root.Entry, extraUpds, types.NewUpdateInsertFlags(), "bench", 5); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
