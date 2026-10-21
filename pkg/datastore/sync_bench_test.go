package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	protoImporter "github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// Benchmarks that attribute the cost of a sync (ApplyToRunning) to its stages.
//
// Run with e.g.:
//
//	go test ./pkg/datastore -run '^$' -bench . -benchmem -benchtime=5x
//
// Baseline numbers (16 threads, mocked cache and target, n=10_000) are recorded in
// .scratch/sync-performance-and-drift-revert/BASELINE.md.
//
// Sizes are the number of interfaces; every interface carries 4 subinterfaces,
// resulting in roughly 11 leaf entries per interface.

const (
	benchIntentName = "bench-intent"
	benchIntentPrio = int32(10)
	benchDeleteName = "bench-delete-intent"
	// just above running (lower number = higher precedence)
	benchDeletePrio = consts.RunningValuesPrio - 1
)

var benchSizes = []int{100, 1000, 10000}

var (
	benchSchemaOnce sync.Once
	benchSCB        schemaClient.SchemaClientBound
	benchSchemaErr  error
)

func benchSchema(b *testing.B) schemaClient.SchemaClientBound {
	b.Helper()
	benchSchemaOnce.Do(func() {
		sc, schema, err := testhelper.InitSDCIOSchema()
		if err != nil {
			benchSchemaErr = err
			return
		}
		benchSCB = schemaClient.NewSchemaClientBound(schema, sc)
	})
	if benchSchemaErr != nil {
		b.Fatal(benchSchemaErr)
	}
	return benchSCB
}

func benchPool(ctx context.Context) *pool.SharedTaskPool {
	return pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
}

func benchNewRoot(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool) *tree.RootEntry {
	b.Helper()
	tc := tree.NewTreeContext(benchSchema(b), tp)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		b.Fatal(err)
	}
	return root
}

// benchConfig generates the json representation of a config with n interfaces.
func benchConfig(b *testing.B, n int) any {
	b.Helper()
	return benchDeviceJSON(b, benchConfigDevice(n, false))
}

// benchConfigDrifted is like benchConfig but with one leaf changed on the device.
func benchConfigDrifted(b *testing.B, n int) any {
	b.Helper()
	return benchDeviceJSON(b, benchConfigDevice(n, true))
}

func benchConfigDevice(n int, driftFirstDescription bool) *sdcio_schema.Device {
	d := &sdcio_schema.Device{Interface: map[string]*sdcio_schema.SdcioModel_Interface{}}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("ethernet-%d/%d", i/128+1, i%128+1)
		desc := fmt.Sprintf("description of %s", name)
		if driftFirstDescription && i == 0 {
			desc = "drifted-on-device"
		}
		itf := &sdcio_schema.SdcioModel_Interface{
			Name:         ygot.String(name),
			Description:  ygot.String(desc),
			Mtu:          ygot.Uint16(9000),
			Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{},
		}
		for s := uint32(0); s < 4; s++ {
			itf.Subinterface[s] = &sdcio_schema.SdcioModel_Interface_Subinterface{
				Index:       ygot.Uint32(s),
				Description: ygot.String(fmt.Sprintf("sub %d of %s", s, name)),
			}
		}
		d.Interface[name] = itf
	}
	return d
}

func benchDeviceJSON(b *testing.B, d *sdcio_schema.Device) any {
	b.Helper()
	s, err := ygot.EmitJSON(d, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		b.Fatal(err)
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		b.Fatal(err)
	}
	return v
}

func benchRunningImporter(v any) *jsonImporter.JsonTreeImporter {
	return jsonImporter.NewJsonTreeImporter(v, consts.RunningIntentName, consts.RunningValuesPrio, false)
}

// benchIntent builds a persisted intent that matches the config exactly.
func benchIntent(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool, v any) *tree_persist.Intent {
	b.Helper()
	root := benchNewRoot(b, ctx, tp)
	_, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(v, benchIntentName, benchIntentPrio, false), treetypes.NewUpdateInsertFlags(), tp)
	if err != nil {
		b.Fatal(err)
	}
	intent, err := ops.TreeExport(root.Entry, benchIntentName, benchIntentPrio, false)
	if err != nil {
		b.Fatal(err)
	}
	return intent
}

// benchDeletePathIntent is a delete-path intent on / (no config values).
func benchDeletePathIntent(b *testing.B, _ context.Context, _ *pool.SharedTaskPool) *tree_persist.Intent {
	b.Helper()
	return &tree_persist.Intent{
		IntentName:      benchDeleteName,
		Priority:        benchDeletePrio,
		ExplicitDeletes: []*sdcpb.Path{{}},
	}
}

func benchIntentsWithDeletePath(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool, deletePathRoot bool, intents ...*tree_persist.Intent) []*tree_persist.Intent {
	b.Helper()
	if !deletePathRoot {
		return intents
	}
	out := make([]*tree_persist.Intent, len(intents), len(intents)+1)
	copy(out, intents)
	return append(out, benchDeletePathIntent(b, ctx, tp))
}

func benchAddDeletePathCoverage(root *tree.RootEntry) {
	ps := sdcpb.NewPathSet()
	ps.AddPaths([]*sdcpb.Path{{}})
	root.GetTreeContext().ExplicitDeletes().Add(benchDeleteName, benchDeletePrio, ps)
}

func benchDatastore(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool, syncTree *tree.RootEntry, intents ...*tree_persist.Intent) *Datastore {
	b.Helper()
	ctrl := gomock.NewController(b)
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
			for _, i := range intents {
				intentChan <- i
			}
			close(intentChan)
			close(errChan)
		}).AnyTimes()
	sbi := mocktarget.NewMockTarget(ctrl)
	sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).AnyTimes()
	return &Datastore{
		config: &config.DatastoreConfig{
			Name:       "bench",
			Validation: config.NewValidationConfig(),
		},
		syncTreeMutex: &sync.RWMutex{},
		syncTree:      syncTree,
		taskPool:      tp,
		cacheClient:   ccb,
		sbi:           sbi,
		dmutex:        &sync.Mutex{},
		schemaClient:  benchSchema(b),
	}
}

func benchPopulatedRunning(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool, v any) *tree.RootEntry {
	b.Helper()
	root := benchNewRoot(b, ctx, tp)
	_, err := root.ImportConfig(ctx, &sdcpb.Path{}, benchRunningImporter(v), treetypes.NewUpdateInsertFlags(), tp)
	if err != nil {
		b.Fatal(err)
	}
	return root
}

func benchForDeletePathVariant(b *testing.B, name string, fn func(b *testing.B, deletePathRoot bool)) {
	b.Run(name, func(b *testing.B) { fn(b, false) })
	b.Run(name+"/delete-path-root", func(b *testing.B) { fn(b, true) })
}

func benchReportSyncTreeLockHold(b *testing.B, totalHold time.Duration) {
	b.Helper()
	if b.N == 0 {
		return
	}
	b.ReportMetric(float64(totalHold.Nanoseconds())/float64(b.N), "sync_tree_lock_hold_ns/op")
}

func benchApplyLoop(b *testing.B, ds *Datastore, ctx context.Context, deletes []*sdcpb.Path, imp *jsonImporter.JsonTreeImporter) {
	b.Helper()
	var totalHold time.Duration
	ds.syncTreeLockHoldReporter = func(d time.Duration) { totalHold += d }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ds.ApplyToRunning(ctx, deletes, imp); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	benchReportSyncTreeLockHold(b, totalHold)
}

// BenchmarkSyncEndToEnd measures the whole ApplyToRunning, including the revert stage.
func BenchmarkSyncEndToEnd(b *testing.B) {
	ctx := context.Background()
	for _, n := range benchSizes {
		v := benchConfig(b, n)
		tp := benchPool(ctx)
		intent := benchIntent(b, ctx, tp, v)

		benchForDeletePathVariant(b, fmt.Sprintf("steady-no-intents/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot)
			ds := benchDatastore(b, ctx, tp, benchPopulatedRunning(b, ctx, tp, v), intents...)
			benchApplyLoop(b, ds, ctx, []*sdcpb.Path{{}}, benchRunningImporter(v))
		})

		benchForDeletePathVariant(b, fmt.Sprintf("steady-matching-intent/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
			ds := benchDatastore(b, ctx, tp, benchPopulatedRunning(b, ctx, tp, v), intents...)
			benchApplyLoop(b, ds, ctx, []*sdcpb.Path{{}}, benchRunningImporter(v))
		})

		benchForDeletePathVariant(b, fmt.Sprintf("first-sync-no-intents/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot)
			b.ReportAllocs()
			var totalHold time.Duration
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				ds := benchDatastore(b, ctx, tp, benchNewRoot(b, ctx, tp), intents...)
				ds.syncTreeLockHoldReporter = func(d time.Duration) { totalHold += d }
				b.StartTimer()
				if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, benchRunningImporter(v)); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			benchReportSyncTreeLockHold(b, totalHold)
		})

		benchForDeletePathVariant(b, fmt.Sprintf("first-sync-matching-intent/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
			b.ReportAllocs()
			var totalHold time.Duration
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				ds := benchDatastore(b, ctx, tp, benchNewRoot(b, ctx, tp), intents...)
				ds.syncTreeLockHoldReporter = func(d time.Duration) { totalHold += d }
				b.StartTimer()
				if err := ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, benchRunningImporter(v)); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			benchReportSyncTreeLockHold(b, totalHold)
		})
	}
}

// BenchmarkSyncDriftRevert exercises the real Drift revert path (performRevert → target Set)
// after one leaf changed on the device.
func BenchmarkSyncDriftRevert(b *testing.B) {
	ctx := context.Background()
	const n = 10000
	tp := benchPool(ctx)
	v := benchConfig(b, n)
	drifted := benchConfigDrifted(b, n)
	intent := benchIntent(b, ctx, tp, v)

	benchForDeletePathVariant(b, fmt.Sprintf("one-leaf-drift/n=%d", n), func(b *testing.B, deletePathRoot bool) {
		intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
		ds := benchDatastore(b, ctx, tp, benchPopulatedRunning(b, ctx, tp, v), intents...)
		benchApplyLoop(b, ds, ctx, []*sdcpb.Path{{}}, benchRunningImporter(drifted))
	})
}

// BenchmarkAddUpdatesRecursive measures adding a single interface worth of values (path by path,
// as the gNMI sync would do it) into trees of different width, to see how it scales with the list size.
func BenchmarkAddUpdatesRecursive(b *testing.B) {
	ctx := context.Background()
	tp := benchPool(ctx)
	one := benchConfig(b, 1)

	scratch := benchPopulatedRunning(b, ctx, tp, one)
	updates := treetypes.UpdateSlice(api.LeafEntriesToUpdates(ops.LeafsOfOwner(scratch.Entry, consts.RunningIntentName))).ToPathAndUpdateSlice()
	if len(updates) == 0 {
		b.Fatal("no updates extracted")
	}
	b.Logf("updates per interface: %d", len(updates))

	for _, n := range []int{100, 1000, 10000} {
		benchForDeletePathVariant(b, fmt.Sprintf("into-tree-with-%d-interfaces", n), func(b *testing.B, deletePathRoot bool) {
			target := benchPopulatedRunning(b, ctx, tp, benchConfig(b, n))
			if deletePathRoot {
				benchAddDeletePathCoverage(target)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := target.AddUpdatesRecursive(ctx, updates, treetypes.NewUpdateInsertFlags()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSyncScoped measures a sync that only covers k of the n interfaces of a large tree
// (e.g. a frequently polled subtree next to a large, rarely synced rest).
func BenchmarkSyncScoped(b *testing.B) {
	ctx := context.Background()
	const n = 10000
	full := benchConfig(b, n)
	tp := benchPool(ctx)
	intent := benchIntent(b, ctx, tp, full)

	for _, k := range []int{1, 100} {
		sub := benchConfig(b, k)
		paths := make([]*sdcpb.Path, 0, k)
		for i := 0; i < k; i++ {
			name := fmt.Sprintf("ethernet-%d/%d", i/128+1, i%128+1)
			paths = append(paths, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": name})}})
		}

		for _, withIntent := range []bool{false, true} {
			base := fmt.Sprintf("scope=%d-of-%d/intent=%t", k, n, withIntent)
			benchForDeletePathVariant(b, base, func(b *testing.B, deletePathRoot bool) {
				var intents []*tree_persist.Intent
				if withIntent {
					intents = benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
				} else {
					intents = benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot)
				}
				ds := benchDatastore(b, ctx, tp, benchPopulatedRunning(b, ctx, tp, full), intents...)
				benchApplyLoop(b, ds, ctx, paths, benchRunningImporter(sub))
			})
		}
	}

	sub := benchConfig(b, 1)
	benchForDeletePathVariant(b, fmt.Sprintf("stream-tick-1-interface-of-%d", n), func(b *testing.B, deletePathRoot bool) {
		intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot)
		ds := benchDatastore(b, ctx, tp, benchPopulatedRunning(b, ctx, tp, full), intents...)
		benchApplyLoop(b, ds, ctx, nil, benchRunningImporter(sub))
	})
}

func benchStageFinishedTree(b *testing.B, ctx context.Context, tp *pool.SharedTaskPool, ds *Datastore, running *tree.RootEntry, deletePathRoot bool) *tree.RootEntry {
	b.Helper()
	finished, err := running.DeepCopy(ctx)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := ds.LoadAllButRunningIntents(ctx, finished); err != nil {
		b.Fatal(err)
	}
	if deletePathRoot {
		benchAddDeletePathCoverage(finished)
	}
	if err := finished.FinishInsertionPhase(ctx); err != nil {
		b.Fatal(err)
	}
	return finished
}

// BenchmarkSyncStages measures the individual stages of a steady state sync.
func BenchmarkSyncStages(b *testing.B) {
	ctx := context.Background()
	for _, n := range benchSizes {
		v := benchConfig(b, n)
		tp := benchPool(ctx)
		intent := benchIntent(b, ctx, tp, v)

		benchForDeletePathVariant(b, fmt.Sprintf("import-steady/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := running.ImportConfig(ctx, &sdcpb.Path{}, benchRunningImporter(v), treetypes.NewUpdateInsertFlags(), tp); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("import-first/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				root := benchNewRoot(b, ctx, tp)
				if deletePathRoot {
					benchAddDeletePathCoverage(root)
				}
				b.StartTimer()
				if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, benchRunningImporter(v), treetypes.NewUpdateInsertFlags(), tp); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("mark-delete-root/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p := processors.NewOwnerDeleteMarker(&processors.OwnerDeleteMarkerProcessorParams{Owner: consts.RunningIntentName})
				if err := p.Run(running.Entry, tp); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				if _, err := running.ImportConfig(ctx, &sdcpb.Path{}, benchRunningImporter(v), treetypes.NewUpdateInsertFlags(), tp); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("remove-deleted-nothing-flagged/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p := processors.NewRemoveDeletedProcessor(&processors.RemoveDeletedProcessorParams{Owner: consts.RunningIntentName})
				if err := p.Run(running.Entry, tp); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("reset-flags/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p := processors.NewResetFlagsProcessor(&processors.ResetFlagsProcessorParams{DeleteFlag: true, NewFlag: true, UpdateFlag: true})
				if err := p.Run(running.Entry, tp); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("deepcopy/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := running.DeepCopy(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("load-intents/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
			ds := benchDatastore(b, ctx, tp, running, intents...)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				cp, err := running.DeepCopy(ctx)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := ds.LoadAllButRunningIntents(ctx, cp); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("finish-insertion/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
			ds := benchDatastore(b, ctx, tp, running, intents...)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				cp, err := running.DeepCopy(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := ds.LoadAllButRunningIntents(ctx, cp); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := cp.FinishInsertionPhase(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("compare-deletes-and-updates/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			intents := benchIntentsWithDeletePath(b, ctx, tp, deletePathRoot, intent)
			ds := benchDatastore(b, ctx, tp, running, intents...)
			finished := benchStageFinishedTree(b, ctx, tp, ds, running, deletePathRoot)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := finished.GetDeletes(true); err != nil {
					b.Fatal(err)
				}
				if _, err := ops.ToProtoUpdates(ctx, finished.Entry, true); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("gnmi-tree-export/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := ops.TreeExport(running.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false); err != nil {
					b.Fatal(err)
				}
			}
		})

		benchForDeletePathVariant(b, fmt.Sprintf("gnmi-proto-import-steady/n=%d", n), func(b *testing.B, deletePathRoot bool) {
			running := benchPopulatedRunning(b, ctx, tp, v)
			if deletePathRoot {
				benchAddDeletePathCoverage(running)
			}
			exported, err := ops.TreeExport(running.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := running.ImportConfig(ctx, &sdcpb.Path{}, protoImporter.NewProtoTreeImporter(exported), treetypes.NewUpdateInsertFlags(), tp); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
