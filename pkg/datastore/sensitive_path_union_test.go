package datastore

import (
	"context"
	"runtime"
	"sync"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/sensitive"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	tree_persist "github.com/sdcio/sdc-protos/tree_persist"
	"go.uber.org/mock/gomock"
)

// buildTestIntent builds a tree_persist.Intent from a device struct using
// TreeExport, then stamps the provided sensitive_paths onto it. If device is
// nil the Root will be empty (useful for marker-only intents).
func buildTestIntent(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, device *sdcio_schema.Device, intentName string, prio int32, sensitivePaths []*sdcpb.Path) *tree_persist.Intent {
	t.Helper()

	flagsNew := treetypes.NewUpdateInsertFlags()
	flagsNew.SetNewFlag()

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	if device != nil {
		_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, device, root.Entry, intentName, prio, false, flagsNew)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	if device == nil {
		return &tree_persist.Intent{
			IntentName:     intentName,
			Priority:       prio,
			SensitivePaths: sensitivePaths,
		}
	}

	intent, err := ops.TreeExport(root.Entry, intentName, prio, false)
	if err != nil {
		t.Fatalf("TreeExport failed: %v", err)
	}
	intent.SensitivePaths = sensitivePaths
	return intent
}

// buildEmptySyncTree returns a RootEntry with an empty (finished) syncTree,
// suitable for use as the datastore syncTree in tests.
func buildEmptySyncTree(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound) *tree.RootEntry {
	t.Helper()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestBlameConfig_CrossIntentSensitivePathRedaction: when another intent marks
// /patterntest sensitive, BlameConfig redacts even if the schema and owning
// intent do not — include_sensitive reveals the real value.
func TestBlameConfig_CrossIntentSensitivePathRedaction(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	// Plain schema client — patterntest is NOT schema-sensitive.
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// data-intent: holds the actual patterntest value, no sensitive_paths.
	dataDevice := &sdcio_schema.Device{Patterntest: ygot.String("secret-value")}
	dataIntent := buildTestIntent(t, ctx, scb, dataDevice, "data-intent", 10, nil)

	// marker-intent: no data, but marks /patterntest sensitive.
	markerIntent := buildTestIntent(t, ctx, scb, nil, "marker-intent", 5, []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "patterntest"}}, IsRootBased: true}})

	tests := []struct {
		name            string
		exposeSensitive bool
		wantValue       string
	}{
		{
			name:            "redacts path-marker sensitive value when include_sensitive=false",
			exposeSensitive: false,
			wantValue:       "***",
		},
		{
			name:            "reveals path-marker sensitive value when include_sensitive=true",
			exposeSensitive: true,
			wantValue:       "secret-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
			// LoadAllButRunningIntents streams via IntentGetAll (excludes running).
			ccb.EXPECT().
				IntentGetAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ []string, intentChan chan<- *tree_persist.Intent, errChan chan<- error) {
					intentChan <- dataIntent
					intentChan <- markerIntent
					close(intentChan)
					close(errChan)
				})

			idx := sensitive.NewPathIndex()
			idx.Set("marker-intent", markerIntent.GetSensitivePaths())

			syncTree := buildEmptySyncTree(t, ctx, scb)
			ds := &Datastore{
				config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
				syncTree:           syncTree,
				syncTreeMutex:      &sync.RWMutex{},
				taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
				cacheClient:        ccb,
				schemaClient:       scb,
				sensitivePathIndex: idx,
			}

			bte, err := ds.BlameConfig(ctx, false, tt.exposeSensitive)
			if err != nil {
				t.Fatalf("BlameConfig() error: %v", err)
			}

			// Walk the blame tree to find "patterntest".
			var patterntestValue string
			found := false
			for _, child := range bte.GetChilds() {
				if child.GetName() == "patterntest" {
					found = true
					patterntestValue = child.GetValue().GetStringVal()
					break
				}
			}
			if !found {
				t.Fatal("patterntest not found in blame tree")
			}
			if patterntestValue != tt.wantValue {
				t.Errorf("patterntest value = %q, want %q", patterntestValue, tt.wantValue)
			}
		})
	}
}

// TestGetIntent_CrossIntentPathsRedact: another intent's path markers in the
// live index redact GetIntent output for a named intent when include_sensitive=false.
func TestGetIntent_CrossIntentPathsRedact(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	// Plain schema client — patterntest is NOT schema-sensitive.
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// data-intent: holds the actual patterntest value, no sensitive_paths.
	dataDevice := &sdcio_schema.Device{Patterntest: ygot.String("secret-value")}
	dataIntent := buildTestIntent(t, ctx, scb, dataDevice, "data-intent", 10, nil)

	markerSensitivePaths := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "patterntest"}}, IsRootBased: true}}

	tests := []struct {
		name            string
		exposeSensitive bool
		wantValue       string
	}{
		{
			name:            "cross-intent marker redacts when include_sensitive=false",
			exposeSensitive: false,
			wantValue:       "***",
		},
		{
			name:            "cross-intent marker revealed when include_sensitive=true",
			exposeSensitive: true,
			wantValue:       "secret-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
			// GetIntent loads the specific intent only — no IntentGetAll.
			ccb.EXPECT().
				IntentGet(gomock.Any(), "data-intent").
				Return(dataIntent, nil)

			idx := sensitive.NewPathIndex()
			idx.Set("marker-intent", markerSensitivePaths)

			syncTree := buildEmptySyncTree(t, ctx, scb)
			ds := &Datastore{
				config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
				syncTree:           syncTree,
				syncTreeMutex:      &sync.RWMutex{},
				taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
				cacheClient:        ccb,
				schemaClient:       scb,
				sensitivePathIndex: idx,
			}

			resp, err := ds.GetIntent(ctx, "data-intent", tt.exposeSensitive)
			if err != nil {
				t.Fatalf("GetIntent() error: %v", err)
			}

			updates, err := resp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("ToProtoUpdates() error: %v", err)
			}

			for _, u := range updates {
				elems := u.GetPath().GetElem()
				if len(elems) > 0 && elems[len(elems)-1].GetName() == "patterntest" {
					got := u.GetValue().GetStringVal()
					if got != tt.wantValue {
						t.Errorf("patterntest value = %q, want %q", got, tt.wantValue)
					}
					return
				}
			}
			t.Errorf("patterntest update not found in GetIntent response; all updates: %v", updates)
		})
	}
}

// TestGetIntent_OwnSensitivePaths_Redacted verifies that an intent's own
// sensitive_paths markers DO cause redaction in GetIntent for that intent.
func TestGetIntent_OwnSensitivePaths_Redacted(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// data-intent: holds patterntest value AND marks it sensitive itself.
	dataDevice := &sdcio_schema.Device{Patterntest: ygot.String("my-secret")}
	ownSensitivePaths := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "patterntest"}}, IsRootBased: true}}
	dataIntent := buildTestIntent(t, ctx, scb, dataDevice, "data-intent", 10, ownSensitivePaths)

	tests := []struct {
		name            string
		exposeSensitive bool
		wantValue       string
	}{
		{
			name:            "own sensitive path is redacted when include_sensitive=false",
			exposeSensitive: false,
			wantValue:       "***",
		},
		{
			name:            "own sensitive path is revealed when include_sensitive=true",
			exposeSensitive: true,
			wantValue:       "my-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
			ccb.EXPECT().
				IntentGet(gomock.Any(), "data-intent").
				Return(dataIntent, nil)

			syncTree := buildEmptySyncTree(t, ctx, scb)
			idx := sensitive.NewPathIndex()
			idx.Set("data-intent", ownSensitivePaths)
			ds := &Datastore{
				config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
				syncTree:           syncTree,
				syncTreeMutex:      &sync.RWMutex{},
				taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
				cacheClient:        ccb,
				schemaClient:       scb,
				sensitivePathIndex: idx,
			}

			resp, err := ds.GetIntent(ctx, "data-intent", tt.exposeSensitive)
			if err != nil {
				t.Fatalf("GetIntent() error: %v", err)
			}
			updates, err := resp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("ToProtoUpdates() error: %v", err)
			}

			for _, u := range updates {
				elems := u.GetPath().GetElem()
				if len(elems) > 0 && elems[len(elems)-1].GetName() == "patterntest" {
					got := u.GetValue().GetStringVal()
					if got != tt.wantValue {
						t.Errorf("patterntest value = %q, want %q", got, tt.wantValue)
					}
					return
				}
			}
			t.Errorf("patterntest update not found in GetIntent response")
		})
	}
}

// TestGetIntent_NoSensitivePaths_NoRedaction verifies that an intent with no
// sensitive_paths does not cause redaction via the path-marker mechanism
// (schema-flag redaction is tested separately in the ops package).
func TestGetIntent_NoSensitivePaths_NoRedaction(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// data-intent: holds patterntest, no sensitive_paths.
	dataDevice := &sdcio_schema.Device{Patterntest: ygot.String("plain-value")}
	dataIntent := buildTestIntent(t, ctx, scb, dataDevice, "data-intent", 10, nil)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)
	ccb.EXPECT().
		IntentGet(gomock.Any(), "data-intent").
		Return(dataIntent, nil)

	syncTree := buildEmptySyncTree(t, ctx, scb)
	ds := &Datastore{
		config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
		syncTree:           syncTree,
		syncTreeMutex:      &sync.RWMutex{},
		taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		cacheClient:        ccb,
		schemaClient:       scb,
		sensitivePathIndex: sensitive.NewPathIndex(),
	}

	resp, err := ds.GetIntent(ctx, "data-intent", false)
	if err != nil {
		t.Fatalf("GetIntent() error: %v", err)
	}
	updates, err := resp.ToProtoUpdates(ctx)
	if err != nil {
		t.Fatalf("ToProtoUpdates() error: %v", err)
	}

	for _, u := range updates {
		elems := u.GetPath().GetElem()
		if len(elems) > 0 && elems[len(elems)-1].GetName() == "patterntest" {
			got := u.GetValue().GetStringVal()
			if got != "plain-value" {
				t.Errorf("patterntest value = %q, want %q (no redaction expected)", got, "plain-value")
			}
			return
		}
	}
	t.Error("patterntest update not found in GetIntent response")
}

// TestGetIntent_RunningIntent_CrossIntentRedaction verifies that fetching the
// running intent applies the full cross-intent union from the sensitivePathIndex.
// The running intent has no own markers; any intent's classification must apply
// to prevent device-echoed plaintext values from leaking.
func TestGetIntent_RunningIntent_CrossIntentRedaction(t *testing.T) {
	ctx := context.Background()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	// marker-intent marks /patterntest sensitive — pre-loaded into index.
	markerSensitivePaths := []*sdcpb.Path{{Elem: []*sdcpb.PathElem{{Name: "patterntest"}}, IsRootBased: true}}
	idx := sensitive.NewPathIndex()
	idx.Set("marker-intent", markerSensitivePaths)

	// Build syncTree with a running value for patterntest.
	flagsExisting := treetypes.NewUpdateInsertFlags()
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	syncRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	runningDevice := &sdcio_schema.Device{Patterntest: ygot.String("running-secret")}
	_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, runningDevice, syncRoot.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false, flagsExisting)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// GetIntent(running) does not call IntentGetAll — it reads from the index.
	ccb := mockcacheclient.NewMockCacheClientBound(ctrl)

	ds := &Datastore{
		config:             &config.DatastoreConfig{Name: "test-ds", Validation: config.NewValidationConfig()},
		syncTree:           syncRoot,
		syncTreeMutex:      &sync.RWMutex{},
		taskPool:           pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		cacheClient:        ccb,
		schemaClient:       scb,
		sensitivePathIndex: idx,
	}

	resp, err := ds.GetIntent(ctx, consts.RunningIntentName, false)
	if err != nil {
		t.Fatalf("GetIntent(running) error: %v", err)
	}
	updates, err := resp.ToProtoUpdates(ctx)
	if err != nil {
		t.Fatalf("ToProtoUpdates() error: %v", err)
	}

	for _, u := range updates {
		elems := u.GetPath().GetElem()
		if len(elems) > 0 && elems[len(elems)-1].GetName() == "patterntest" {
			got := u.GetValue().GetStringVal()
			if got != "***" {
				t.Errorf("running intent patterntest = %q, want %q (running value should be redacted)", got, "***")
			}
			return
		}
	}
	t.Error("patterntest update not found in running intent response")
}
