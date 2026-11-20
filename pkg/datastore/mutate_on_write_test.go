// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	cacheconfig "github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

type mowDatastoreFixture struct {
	ds         *Datastore
	sbi        *mocktarget.MockTarget
	localCache cache.Client
	schema     *sdcpb.Schema
}

func initMutateOnWriteSchema(t *testing.T) (schema.Client, *config.SchemaConfig) {
	t.Helper()
	yangDir := t.TempDir()

	files := map[string]string{
		"sdcio-ext.yang": `module sdcio-ext {
  yang-version 1.1;
  namespace "urn:sdcio-ext";
  prefix "sdcio-ext";
  extension mutate-on-write;
}
`,
		"iana-crypt-hash.yang": `module iana-crypt-hash {
  namespace "urn:ietf:params:xml:ns:yang:iana-crypt-hash";
  prefix "ianach";
  typedef crypt-hash {
    type string;
  }
}
`,
		"device.yang": `module ds-mow-test {
  yang-version 1.1;
  namespace "urn:ds:mow:test";
  prefix "dsmow";

  import iana-crypt-hash { prefix "ianach"; }

  container system {
    leaf hashed-password {
      type ianach:crypt-hash;
    }
    leaf overlay-password {
      type string {
        length "8..128";
      }
      must "string-length(.) <= 32" {
        error-message "cleartext overlay-password must be at most 32 chars";
      }
    }
    leaf-list hashed-key {
      type ianach:crypt-hash;
    }
    leaf hostname {
      type string;
    }
    list user {
      key "username";
      leaf username {
        type string;
      }
      leaf password {
        type ianach:crypt-hash;
      }
    }
  }
}
`,
		"overlay.yang": `module ds-mow-overlay {
  yang-version 1.1;
  namespace "urn:ds:mow:overlay";
  prefix "dsov";

  import ds-mow-test { prefix "dsmow"; }
  import sdcio-ext   { prefix "mowext"; }

  deviation /dsmow:system/dsmow:overlay-password {
    deviate add {
      mowext:mutate-on-write;
    }
  }
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(yangDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	st := memstore.New()
	t.Cleanup(func() { _ = st.Close() })

	sc := schema.NewLocalClient(st)
	scRef := &sdcpb.Schema{
		Name:    "ds-mow",
		Vendor:  "test",
		Version: "1.0.0",
	}
	if _, err := sc.CreateSchema(context.Background(), &sdcpb.CreateSchemaRequest{
		Schema: scRef,
		File:   []string{yangDir},
	}); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	return sc, &config.SchemaConfig{
		Name:    scRef.Name,
		Vendor:  scRef.Vendor,
		Version: scRef.Version,
	}
}

func newMOWDatastoreFixture(t *testing.T, ctrl *gomock.Controller) *mowDatastoreFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	sc, schemaCfg := initMutateOnWriteSchema(t)
	scb := schemaClient.NewSchemaClientBound(schemaCfg, sc)

	localCache, err := cache.NewLocalCache(&cacheconfig.CacheConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewLocalCache: %v", err)
	}
	const dsName = "mow-ds"
	if err := localCache.InstanceCreate(ctx, dsName); err != nil {
		t.Fatalf("InstanceCreate: %v", err)
	}
	ccb := cache.NewCacheClientBound(dsName, localCache)

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	confirmed := newConfirmedAppliedValues()
	tc := tree.NewTreeContext(scb, vpf, tree.WithConfirmedAppliedValues(confirmed))
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("FinishInsertionPhase: %v", err)
	}

	sbi := mocktarget.NewMockTarget(ctrl)
	ds := &Datastore{
		config: &config.DatastoreConfig{
			Name:       dsName,
			Schema:     schemaCfg,
			Validation: config.NewValidationConfig(),
			SBI:        &config.SBI{Type: "gnmi", GnmiOptions: &config.SBIGnmiOptions{Encoding: "JSON_IETF"}},
		},
		cacheClient:      ccb,
		sbi:              sbi,
		schemaClient:     scb,
		ctx:              ctx,
		cfn:              cancel,
		m:                &sync.RWMutex{},
		deviationClients: make(map[sdcpb.DataServer_WatchDeviationsServer]string),
		dmutex:           &sync.Mutex{},
		syncTree:         syncTreeRoot,
		syncTreeMutex:    &sync.RWMutex{},
		confirmedApplied: confirmed,
		taskPool:         vpf,
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds), types.WithAutoRollbackLock(ds.dmutex))

	return &mowDatastoreFixture{
		ds:         ds,
		sbi:        sbi,
		localCache: localCache,
		schema: &sdcpb.Schema{
			Name:    schemaCfg.Name,
			Vendor:  schemaCfg.Vendor,
			Version: schemaCfg.Version,
		},
	}
}

func buildJSONIntent(t *testing.T, ds *Datastore, name string, prio int32, nonRevertive bool, payload map[string]any) *types.TransactionIntent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ti, err := ds.SdcpbTransactionIntentToInternalTI(context.Background(), &sdcpb.TransactionIntent{
		Intent:       name,
		Priority:     prio,
		NonRevertive: nonRevertive,
		Update: []*sdcpb.Update{{
			Path: &sdcpb.Path{},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: raw},
			},
		}},
	})
	if err != nil {
		t.Fatalf("SdcpbTransactionIntentToInternalTI: %v", err)
	}
	return ti
}

type fakeWatchDeviationStream struct {
	grpc.ServerStream
	ctx    context.Context
	mu     sync.Mutex
	events []*sdcpb.WatchDeviationResponse
	doneCh chan struct{}
	once   sync.Once
}

func newFakeWatchDeviationStream(ctx context.Context) *fakeWatchDeviationStream {
	pCtx := peer.NewContext(ctx, &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321},
	})
	return &fakeWatchDeviationStream{
		ctx:    pCtx,
		doneCh: make(chan struct{}),
	}
}

func (f *fakeWatchDeviationStream) Context() context.Context {
	return f.ctx
}

func (f *fakeWatchDeviationStream) Send(resp *sdcpb.WatchDeviationResponse) error {
	f.mu.Lock()
	f.events = append(f.events, resp)
	f.mu.Unlock()
	if resp.GetEvent() == sdcpb.DeviationEvent_END {
		f.once.Do(func() { close(f.doneCh) })
	}
	return nil
}

func collectWatchDeviationsBatch(t *testing.T, ds *Datastore) []*sdcpb.WatchDeviationResponse {
	t.Helper()
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	defer mgrCancel()

	stream := newFakeWatchDeviationStream(mgrCtx)
	if err := ds.WatchDeviations(&sdcpb.WatchDeviationRequest{Name: []string{ds.Name()}}, stream); err != nil {
		t.Fatalf("WatchDeviations: %v", err)
	}
	defer ds.StopDeviationsWatch(stream)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ds.DeviationMgr(mgrCtx, &config.DeviationConfig{Interval: 5 * time.Millisecond})
	}()

	select {
	case <-stream.doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WatchDeviations batch")
	}
	mgrCancel()
	<-done

	stream.mu.Lock()
	defer stream.mu.Unlock()
	return append([]*sdcpb.WatchDeviationResponse(nil), stream.events...)
}

func updatesByXPath(upds []*sdcpb.Update) map[string]string {
	m := make(map[string]string, len(upds))
	for _, u := range upds {
		p := u.GetPath().ToXPath(false)
		if len(p) > 0 && p[0] != '/' {
			p = "/" + p
		}
		m[p] = u.GetValue().ToString()
	}
	return m
}

func expectEmptySet(t *testing.T, sbi *mocktarget.MockTarget) *gomock.Call {
	t.Helper()
	return sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			upds, err := source.ToProtoUpdates(setCtx, true)
			if err != nil {
				return nil, err
			}
			dels, err := source.ToProtoDeletes(setCtx)
			if err != nil {
				return nil, err
			}
			if len(upds) != 0 || len(dels) != 0 {
				t.Fatalf("expected empty southbound diff, got upds=%v dels=%v", upds, dels)
			}
			return &sdcpb.SetDataResponse{}, nil
		}).Times(1)
}

func TestConfirmedAppliedValues_CanonicalMultiKeyPrefixSorting(t *testing.T) {
	store := newConfirmedAppliedValues()
	prefixed := &sdcpb.Path{
		Origin:      "openconfig",
		IsRootBased: false,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("zmod:acl", map[string]string{"zmod:a": "corpA:alice", "amod:b": "10.0.0.1:179"}),
			sdcpb.NewPathElem("zmod:secret", nil),
		},
	}
	unprefixed := &sdcpb.Path{
		IsRootBased: true,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("acl", map[string]string{"a": "corpA:alice", "b": "10.0.0.1:179"}),
			sdcpb.NewPathElem("secret", nil),
		},
	}
	distinctColonKey := &sdcpb.Path{
		IsRootBased: true,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("acl", map[string]string{"a": "corpB:alice", "b": "10.0.0.1:179"}),
			sdcpb.NewPathElem("secret", nil),
		},
	}
	intended := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "plain"}}
	confirmed := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$hash"}}

	store.Record(prefixed, "intent1", intended, confirmed)
	got := store.Lookup(unprefixed, "intent1", intended)
	if got == nil || got.GetStringVal() != "$6$hash" {
		t.Fatalf("Lookup across inverted module-prefix multi-key path = %v, want $6$hash", got)
	}
	if collide := store.Lookup(distinctColonKey, "intent1", intended); collide != nil {
		t.Fatalf("single-colon string key corpB:alice collided with corpA:alice: %v", collide)
	}
	if gotXPath := canonicalPath(prefixed).ToXPath(false); gotXPath != "/acl[a=corpA:alice][b=10.0.0.1:179]/secret" {
		t.Fatalf("canonicalPath XPath = %q, want /acl[a=corpA:alice][b=10.0.0.1:179]/secret", gotXPath)
	}
}

func TestDatastore_MutateOnWrite_SingleCommitConvergenceAndReads(t *testing.T) {
	for _, tc := range []struct {
		name         string
		nonRevertive bool
	}{
		{name: "revertive intent", nonRevertive: false},
		{name: "non-revertive intent", nonRevertive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const (
				cleartext = "MyS3cret!"
				hashed    = "$6$rounds=5000$saltsalt$hasheddigestvalue"
				hostname  = "leaf-router-01"
			)

			var pushedUpdates []*sdcpb.Update
			setCall := f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
					var err error
					pushedUpdates, err = source.ToProtoUpdates(setCtx, true)
					if err != nil {
						return nil, err
					}
					return &sdcpb.SetDataResponse{}, nil
				}).Times(1)

			f.sbi.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				After(setCall).
				DoAndReturn(func(getCtx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
					if req.GetDataType() != sdcpb.DataType_CONFIG {
						t.Errorf("Get DataType = %v, want %v", req.GetDataType(), sdcpb.DataType_CONFIG)
					}
					if len(req.GetPath()) != 1 {
						t.Fatalf("expected targeted Get for 1 mutate-on-write path, got %d: %v", len(req.GetPath()), req.GetPath())
					}
					gotPath := req.GetPath()[0].ToXPath(false)
					if gotPath != "/system/hashed-password" && gotPath != "system/hashed-password" {
						t.Errorf("targeted Get path = %q, want /system/hashed-password", gotPath)
					}

					// Concurrent non-dryRun TransactionSet must fail fast while post-Set Get holds dmutex.
					_, lockErr := f.ds.TransactionSet(context.Background(), "concurrent-tx", nil, nil, time.Minute, false)
					if !errors.Is(lockErr, ErrDatastoreLocked) {
						t.Errorf("concurrent TransactionSet during post-Set Get returned %v, want ErrDatastoreLocked", lockErr)
					}

					return &sdcpb.GetDataResponse{
						Notification: []*sdcpb.Notification{{
							Update: []*sdcpb.Update{{
								Path: &sdcpb.Path{
									Elem: []*sdcpb.PathElem{
										sdcpb.NewPathElem("system", nil),
										sdcpb.NewPathElem("hashed-password", nil),
									},
								},
								Value: &sdcpb.TypedValue{
									Value: &sdcpb.TypedValue_StringVal{StringVal: hashed},
								},
							}},
						}},
					}, nil
				}).Times(1)

			intent := buildJSONIntent(t, f.ds, "intent1", 10, tc.nonRevertive, map[string]any{
				"system": map[string]any{
					"hashed-password": cleartext,
					"hostname":        hostname,
				},
			})

			resp, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{intent}, nil, time.Minute, false)
			if err != nil {
				t.Fatalf("TransactionSet: %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
				t.Fatalf("TransactionConfirm: %v", err)
			}

			pushedMap := updatesByXPath(pushedUpdates)
			if pushedMap["/system/hashed-password"] != cleartext || pushedMap["/system/hostname"] != hostname {
				t.Fatalf("pushed southbound updates = %v, want cleartext %q and hostname %q", pushedMap, cleartext, hostname)
			}
			respMap := updatesByXPath(resp.GetUpdate())
			if respMap["/system/hashed-password"] != cleartext {
				t.Fatalf("TransactionSetResponse update for hashed-password = %q, want %q", respMap["/system/hashed-password"], cleartext)
			}

			namedResp, err := f.ds.GetIntent(ctx, "intent1")
			if err != nil {
				t.Fatalf("GetIntent(intent1): %v", err)
			}
			namedUpds, err := namedResp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("namedResp.ToProtoUpdates: %v", err)
			}
			if got := updatesByXPath(namedUpds)["/system/hashed-password"]; got != cleartext {
				t.Fatalf("GetIntent(intent1) hashed-password = %q, want authored %q", got, cleartext)
			}
			persistedBlob, err := f.localCache.InstanceIntentGet(ctx, f.ds.Name(), "intent1")
			if err != nil {
				t.Fatalf("InstanceIntentGet(intent1): %v", err)
			}
			if rawBlob := persistedBlob.String(); rawBlob == "" {
				t.Fatal("expected non-empty persisted named intent blob")
			} else if strings.Contains(rawBlob, hashed) {
				t.Fatalf("persisted named intent blob leaked confirmed applied value %q: %s", hashed, rawBlob)
			}

			runningResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
			if err != nil {
				t.Fatalf("GetIntent(running): %v", err)
			}
			runningUpds, err := runningResp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("runningResp.ToProtoUpdates: %v", err)
			}
			if got := updatesByXPath(runningUpds)["/system/hashed-password"]; got != hashed {
				t.Fatalf("GetIntent(running) hashed-password = %q, want target readback %q", got, hashed)
			}

			syncPayload := map[string]any{
				"system": map[string]any{
					"hashed-password": hashed,
					"hostname":        hostname,
				},
			}
			if err := f.ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(syncPayload, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning: %v", err)
			}

			events := collectWatchDeviationsBatch(t, f.ds)
			for _, ev := range events {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
					t.Fatalf("unexpected NOT_APPLIED deviation on converged mutate-on-write leaf: %v", ev)
				}
			}

			blame, err := f.ds.BlameConfig(ctx, false)
			if err != nil {
				t.Fatalf("BlameConfig: %v", err)
			}
			pwBlame := testhelper.FindBlameElement(blame, "system", "hashed-password")
			if pwBlame == nil {
				t.Fatalf("missing system/hashed-password in BlameConfig: %v", blame)
			}
			if pwBlame.GetOwner() != "intent1" || pwBlame.GetValue().GetStringVal() != cleartext {
				t.Fatalf("BlameConfig owner/value = (%q, %v), want (%q, %q)",
					pwBlame.GetOwner(), pwBlame.GetValue(), "intent1", cleartext)
			}
			if pwBlame.GetDeviationValue() != nil {
				t.Fatalf("expected nil DeviationValue in BlameConfig, got %v", pwBlame.GetDeviationValue())
			}
		})
	}
}

func TestDatastore_MutateOnWrite_NonRepushValidationAndLeafList(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		pwCleartext      = "AdminP@ss1"
		pwHashed         = "$6$rounds=5000$salt1$hash1"
		overlayCleartext = "OverlaySecret1"                                                     // 14 chars (satisfies length 8..32)
		overlayCipher    = "$9$rounds=10000$longsaltvalue$ciphertext_much_longer_than_32_chars" // 66 chars (> 32-char YANG length limit)
	)
	hashedKeys := []string{"$6$salt$keyhash1", "$6$salt$keyhash2"}

	set1 := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)

	f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(set1).
		DoAndReturn(func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
			if len(req.GetPath()) != 3 {
				t.Fatalf("expected targeted Get for 3 mutate-on-write paths, got %d: %v", len(req.GetPath()), req.GetPath())
			}
			pwJSON, _ := json.Marshal(pwHashed)
			ovJSON, _ := json.Marshal(overlayCipher)
			keysJSON, _ := json.Marshal(hashedKeys)
			return &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Origin:      "openconfig",
								IsRootBased: true,
								Elem: []*sdcpb.PathElem{
									sdcpb.NewPathElem("dsmow:system", nil),
									sdcpb.NewPathElem("hashed-password", nil),
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: pwJSON}},
						},
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									sdcpb.NewPathElem("system", nil),
									sdcpb.NewPathElem("overlay-password", nil),
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: ovJSON}},
						},
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									sdcpb.NewPathElem("system", nil),
									sdcpb.NewPathElem("hashed-key", nil),
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: keysJSON}},
						},
					},
				}},
			}, nil
		}).Times(1)

	intent1 := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
		"system": map[string]any{
			"hashed-password":  pwCleartext,
			"overlay-password": overlayCleartext,
			"hashed-key":       []string{"raw-key-1", "raw-key-2"},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-init", []*types.TransactionIntent{intent1}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-init): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-init"); err != nil {
		t.Fatalf("TransactionConfirm(tx-init): %v", err)
	}

	runningInitResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
	if err != nil {
		t.Fatalf("GetIntent(running): %v", err)
	}
	runningInitUpds, err := runningInitResp.ToProtoUpdates(ctx)
	if err != nil {
		t.Fatalf("runningInitResp.ToProtoUpdates: %v", err)
	}
	runningInitMap := updatesByXPath(runningInitUpds)
	if runningInitMap["/system/overlay-password"] != overlayCipher {
		t.Fatalf("Running overlay-password = %q, want 66-char cipher %q", runningInitMap["/system/overlay-password"], overlayCipher)
	}

	// Unrelated TransactionSet validates authored cleartext (not 66-char Running hash) and skips converged MOW leaves.
	var unrelatedPushed []*sdcpb.Update
	f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			var err error
			unrelatedPushed, err = source.ToProtoUpdates(setCtx, true)
			return &sdcpb.SetDataResponse{}, err
		}).Times(1)

	intent2 := buildJSONIntent(t, f.ds, "intent2", 20, false, map[string]any{
		"system": map[string]any{
			"hostname": "rtr-unrelated",
		},
	})
	resp2, err := f.ds.TransactionSet(ctx, "tx-unrelated", []*types.TransactionIntent{intent2}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(tx-unrelated): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-unrelated"); err != nil {
		t.Fatalf("TransactionConfirm(tx-unrelated): %v", err)
	}
	pushedMap2 := updatesByXPath(unrelatedPushed)
	if len(pushedMap2) != 1 || pushedMap2["/system/hostname"] != "rtr-unrelated" {
		t.Fatalf("unrelated TransactionSet pushed %v, want only /system/hostname=rtr-unrelated", pushedMap2)
	}
	if respMap2 := updatesByXPath(resp2.GetUpdate()); len(respMap2) != 1 || respMap2["/system/hostname"] != "rtr-unrelated" {
		t.Fatalf("unrelated TransactionSetResponse updates = %v, want only /system/hostname=rtr-unrelated", respMap2)
	}

	// Re-bundled unchanged revertive intent1 alongside modified intent2 must still not re-push MOW leaves.
	var rebundledPushed []*sdcpb.Update
	f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			var err error
			rebundledPushed, err = source.ToProtoUpdates(setCtx, true)
			return &sdcpb.SetDataResponse{}, err
		}).Times(1)

	intent1Rebundled := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
		"system": map[string]any{
			"hashed-password":  pwCleartext,
			"overlay-password": overlayCleartext,
			"hashed-key":       []string{"raw-key-1", "raw-key-2"},
		},
	})
	intent2Modified := buildJSONIntent(t, f.ds, "intent2", 20, false, map[string]any{
		"system": map[string]any{
			"hostname": "rtr-rebundled",
		},
	})
	resp3, err := f.ds.TransactionSet(ctx, "tx-rebundled", []*types.TransactionIntent{intent1Rebundled, intent2Modified}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(tx-rebundled): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-rebundled"); err != nil {
		t.Fatalf("TransactionConfirm(tx-rebundled): %v", err)
	}
	pushedMap3 := updatesByXPath(rebundledPushed)
	if len(pushedMap3) != 1 || pushedMap3["/system/hostname"] != "rtr-rebundled" {
		t.Fatalf("re-bundled TransactionSet pushed %v, want only /system/hostname=rtr-rebundled", pushedMap3)
	}
	if respMap3 := updatesByXPath(resp3.GetUpdate()); len(respMap3) != 1 || respMap3["/system/hostname"] != "rtr-rebundled" {
		t.Fatalf("re-bundled TransactionSetResponse updates = %v, want only /system/hostname=rtr-rebundled", respMap3)
	}

	// Re-submitting unchanged revertive intent1 alone pushes zero updates/deletes and issues zero Get calls.
	expectEmptySet(t, f.sbi)
	intent1Unchanged := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
		"system": map[string]any{
			"hashed-password":  pwCleartext,
			"overlay-password": overlayCleartext,
			"hashed-key":       []string{"raw-key-1", "raw-key-2"},
		},
	})
	resp4, err := f.ds.TransactionSet(ctx, "tx-idempotent", []*types.TransactionIntent{intent1Unchanged}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(tx-idempotent): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-idempotent"); err != nil {
		t.Fatalf("TransactionConfirm(tx-idempotent): %v", err)
	}
	if len(resp4.GetUpdate()) != 0 {
		t.Fatalf("expected 0 updates on idempotent re-submission, got %v", resp4.GetUpdate())
	}

	// Zero-diff transaction when disconnected (sbi == nil) returns ErrNotConnected.
	f.ds.sbi = nil
	_, disconnErr := f.ds.TransactionSet(ctx, "tx-disconnected", []*types.TransactionIntent{intent1Unchanged}, nil, time.Minute, false)
	if !errors.Is(disconnErr, targettypes.ErrNotConnected) {
		t.Fatalf("disconnected zero-diff TransactionSet err = %v, want ErrNotConnected", disconnErr)
	}
	f.ds.sbi = f.sbi

	// Schema validation rejects authored cleartext violating the 32-char must constraint and the 8-char length minimum.
	tooLongIntent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
		"system": map[string]any{
			"hashed-password":  pwCleartext,
			"overlay-password": "TooLongCleartextPasswordExceeding32Chars",
			"hashed-key":       []string{"raw-key-1", "raw-key-2"},
		},
	})
	invalidResp, err := f.ds.TransactionSet(ctx, "tx-invalid", []*types.TransactionIntent{tooLongIntent}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(tx-invalid) unexpected rpc error: %v", err)
	}
	if errs := invalidResp.GetIntents()["intent1"].GetErrors(); len(errs) == 0 || !strings.Contains(errs[0], "overlay-password") {
		t.Fatalf("expected overlay-password validation error for intent1, got %v", invalidResp)
	}

	_, shortErr := f.ds.SdcpbTransactionIntentToInternalTI(ctx, &sdcpb.TransactionIntent{
		Intent:   "intent1",
		Priority: 10,
		Update: []*sdcpb.Update{{
			Path: &sdcpb.Path{},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(`{"system":{"overlay-password":"short"}}`),
				},
			},
		}},
	})
	if shortErr == nil {
		t.Fatal("expected length constraint failure for 5-char overlay-password")
	}
}

func TestDatastore_MutateOnWrite_BaselineKeySpecificityAndOverruled(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	set1 := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)

	f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(set1).
		DoAndReturn(func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
			if len(req.GetPath()) != 1 || req.GetPath()[0].ToXPath(false) != "/system/user[username=alice]/password" {
				t.Fatalf("unexpected Get paths: %v", req.GetPath())
			}
			return &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									sdcpb.NewPathElem("system", nil),
									sdcpb.NewPathElem("user", map[string]string{"username": "alice"}),
									sdcpb.NewPathElem("username", nil),
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "alice"}},
						},
						{
							Path: &sdcpb.Path{
								Elem: []*sdcpb.PathElem{
									sdcpb.NewPathElem("system", nil),
									sdcpb.NewPathElem("user", map[string]string{"username": "alice"}),
									sdcpb.NewPathElem("password", nil),
								},
							},
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$alice$hashV1"}},
						},
					},
				}},
			}, nil
		}).Times(1)

	highIntent := buildJSONIntent(t, f.ds, "high-owner", 10, false, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "alice", "password": "HighSecretV1"},
			},
		},
	})
	lowIntent := buildJSONIntent(t, f.ds, "low-owner", 20, false, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "alice", "password": "LowSecretV1"},
			},
		},
	})

	if _, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{highIntent, lowIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-1): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
		t.Fatalf("TransactionConfirm(tx-1): %v", err)
	}

	events := collectWatchDeviationsBatch(t, f.ds)
	foundOverruled := false
	for _, ev := range events {
		if ev.GetEvent() != sdcpb.DeviationEvent_UPDATE {
			continue
		}
		if ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
			t.Fatalf("unexpected NOT_APPLIED deviation: %v", ev)
		}
		if ev.GetReason() == sdcpb.DeviationReason_OVERRULED &&
			ev.GetPath().ToXPath(false) == "/system/user[username=alice]/password" {
			foundOverruled = true
			if ev.GetIntent() != "low-owner" ||
				ev.GetExpectedValue().GetStringVal() != "LowSecretV1" ||
				ev.GetCurrentValue().GetStringVal() != "HighSecretV1" {
				t.Fatalf("OVERRULED event = (intent=%q, expected=%v, current=%v), want (low-owner, LowSecretV1, HighSecretV1)",
					ev.GetIntent(), ev.GetExpectedValue(), ev.GetCurrentValue())
			}
		}
	}
	if !foundOverruled {
		t.Fatalf("expected OVERRULED deviation for low-owner on /system/user[username=alice]/password, got %v", events)
	}

	set2 := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)
	f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(set2).
		Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{
						IsRootBased: true,
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("system", nil),
							sdcpb.NewPathElem("user", map[string]string{"username": "alice"}),
							sdcpb.NewPathElem("password", nil),
						},
					},
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$alice$hashV2"}},
				}},
			}},
		}, nil).Times(1)

	highIntentV2 := buildJSONIntent(t, f.ds, "high-owner", 10, false, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "alice", "password": "HighSecretV2"},
			},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-2", []*types.TransactionIntent{highIntentV2}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-2): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-2"); err != nil {
		t.Fatalf("TransactionConfirm(tx-2): %v", err)
	}

	// PreviouslyApplied super-owner changes winningOwner without pushing, proving baseline owner specificity.
	superIntent, err := f.ds.SdcpbTransactionIntentToInternalTI(ctx, &sdcpb.TransactionIntent{
		Intent:            "super-owner",
		Priority:          5,
		NonRevertive:      true,
		PreviouslyApplied: true,
		Update: []*sdcpb.Update{{
			Path: &sdcpb.Path{},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonIetfVal{
					JsonIetfVal: []byte(`{"system":{"user":[{"username":"alice","password":"HighSecretV2"}]}}`),
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("SdcpbTransactionIntentToInternalTI(super-owner): %v", err)
	}
	expectEmptySet(t, f.sbi)
	if _, err := f.ds.TransactionSet(ctx, "tx-3", []*types.TransactionIntent{superIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-3): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-3"); err != nil {
		t.Fatalf("TransactionConfirm(tx-3): %v", err)
	}

	eventsAfterOwnerChange := collectWatchDeviationsBatch(t, f.ds)
	foundNotAppliedForSuper := false
	for _, ev := range eventsAfterOwnerChange {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
			ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
			ev.GetIntent() == "super-owner" &&
			ev.GetPath().ToXPath(false) == "/system/user[username=alice]/password" {
			foundNotAppliedForSuper = true
			if ev.GetExpectedValue().GetStringVal() != "HighSecretV2" {
				t.Fatalf("NOT_APPLIED ExpectedValue = %v, want HighSecretV2", ev.GetExpectedValue())
			}
		}
	}
	if !foundNotAppliedForSuper {
		t.Fatalf("expected NOT_APPLIED for super-owner because baseline belonged to high-owner, got %v", eventsAfterOwnerChange)
	}
}

func TestDatastore_MutateOnWrite_ReplaceTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("pure replace writes readback hash to Running", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		seedIntent := buildJSONIntent(t, f.ds, "seed", 50, false, map[string]any{
			"system": map[string]any{"hostname": "before-replace"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-seed", []*types.TransactionIntent{seedIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-seed): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-seed"); err != nil {
			t.Fatalf("TransactionConfirm(tx-seed): %v", err)
		}
		orphanSeed, err := f.ds.SdcpbTransactionIntentToInternalTI(ctx, &sdcpb.TransactionIntent{
			Intent:   "seed",
			Priority: 50,
			Delete:   true,
			Orphan:   true,
		})
		if err != nil {
			t.Fatalf("orphan seed: %v", err)
		}
		expectEmptySet(t, f.sbi)
		if _, err := f.ds.TransactionSet(ctx, "tx-orphan", []*types.TransactionIntent{orphanSeed}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-orphan): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-orphan"); err != nil {
			t.Fatalf("TransactionConfirm(tx-orphan): %v", err)
		}

		const (
			replaceCleartext = "ReplaceCleartext!"
			replaceHashed    = "$6$rounds=5000$replsalt$replhash"
			overlayCleartext = "ReplaceOverlay1"
			overlayCipher    = "$9$rounds=10000$longsaltvalue$replace_cipher_longer_than_32_chars"
		)

		replaceSet := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		replaceGet := f.sbi.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			After(replaceSet).
			DoAndReturn(func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
				if req.GetDataType() != sdcpb.DataType_CONFIG {
					t.Errorf("replace Get DataType = %v, want CONFIG", req.GetDataType())
				}
				if len(req.GetPath()) != 2 {
					t.Fatalf("replace Get paths = %v, want 2 mutate-on-write paths", req.GetPath())
				}
				return &sdcpb.GetDataResponse{
					Notification: []*sdcpb.Notification{{
						Update: []*sdcpb.Update{
							{
								Path: &sdcpb.Path{
									IsRootBased: true,
									Elem: []*sdcpb.PathElem{
										sdcpb.NewPathElem("system", nil),
										sdcpb.NewPathElem("hashed-password", nil),
									},
								},
								Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: replaceHashed}},
							},
							{
								Path: &sdcpb.Path{
									IsRootBased: true,
									Elem: []*sdcpb.PathElem{
										sdcpb.NewPathElem("system", nil),
										sdcpb.NewPathElem("overlay-password", nil),
									},
								},
								Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: overlayCipher}},
							},
						},
					}},
				}, nil
			}).Times(1)
		expectEmptySet(t, f.sbi).After(replaceGet)

		replaceTI := buildJSONIntent(t, f.ds, consts.ReplaceIntentName, consts.ReplaceValuesPrio, false, map[string]any{
			"system": map[string]any{
				"hashed-password":  replaceCleartext,
				"overlay-password": overlayCleartext,
				"hostname":         "after-replace",
			},
		})

		if _, err := f.ds.TransactionSet(ctx, "tx-replace", nil, replaceTI, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-replace): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-replace"); err != nil {
			t.Fatalf("TransactionConfirm(tx-replace): %v", err)
		}

		runningResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
		if err != nil {
			t.Fatalf("GetIntent(running): %v", err)
		}
		runningUpds, err := runningResp.ToProtoUpdates(ctx)
		if err != nil {
			t.Fatalf("runningResp.ToProtoUpdates: %v", err)
		}
		runMap := updatesByXPath(runningUpds)
		if runMap["/system/hashed-password"] != replaceHashed ||
			runMap["/system/overlay-password"] != overlayCipher ||
			runMap["/system/hostname"] != "after-replace" {
			t.Fatalf("Running after replace = %v, want hashed=%q overlay=%q hostname=after-replace", runMap, replaceHashed, overlayCipher)
		}
	})

	t.Run("replace matching persisted intent binds baseline without second Set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		const (
			sharedCleartext = "SharedSecret!"
			hash1           = "$6$saltA$hashA"
			hash2           = "$6$saltB$hashB"
		)

		initSet := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(initSet).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{
						IsRootBased: true,
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("system", nil),
							sdcpb.NewPathElem("hashed-password", nil),
						},
					},
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: hash1}},
				}},
			}},
		}, nil).Times(1)

		persistedIntent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
			"system": map[string]any{
				"hashed-password": sharedCleartext,
				"hostname":        "same-host",
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-init", []*types.TransactionIntent{persistedIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-init): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-init"); err != nil {
			t.Fatalf("TransactionConfirm(tx-init): %v", err)
		}

		// Replace with the same cleartext binds intent1's baseline so follow-through diff is empty and issues 1 Get.
		replSet := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		replGet := f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(replSet).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{
						IsRootBased: true,
						Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("system", nil),
							sdcpb.NewPathElem("hashed-password", nil),
						},
					},
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: hash2}},
				}},
			}},
		}, nil).Times(1)
		expectEmptySet(t, f.sbi).After(replGet)

		replaceTI := buildJSONIntent(t, f.ds, consts.ReplaceIntentName, consts.ReplaceValuesPrio, false, map[string]any{
			"system": map[string]any{
				"hashed-password": sharedCleartext,
				"hostname":        "same-host",
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-replace-persisted", nil, replaceTI, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-replace-persisted): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-replace-persisted"); err != nil {
			t.Fatalf("TransactionConfirm(tx-replace-persisted): %v", err)
		}
	})
}

func TestDatastore_MutateOnWrite_DryRunAndNoopSkipTargetedGet(t *testing.T) {
	ctx := context.Background()

	t.Run("dryRun skips Set and targeted Get without modifying Running", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		intent := buildJSONIntent(t, f.ds, "dryrun-intent", 10, false, map[string]any{
			"system": map[string]any{
				"hashed-password": "DryRunPassword!",
				"hostname":        "dryrun-host",
			},
		})
		resp, err := f.ds.TransactionSet(ctx, "tx-dryrun", []*types.TransactionIntent{intent}, nil, time.Minute, true)
		if err != nil {
			t.Fatalf("TransactionSet(dryRun): %v", err)
		}
		if got := updatesByXPath(resp.GetUpdate())["/system/hashed-password"]; got != "DryRunPassword!" {
			t.Fatalf("dryRun response hashed-password = %q, want DryRunPassword!", got)
		}
		runningResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
		if err != nil {
			t.Fatalf("GetIntent(running): %v", err)
		}
		runningUpds, err := runningResp.ToProtoUpdates(ctx)
		if err != nil {
			t.Fatalf("runningResp.ToProtoUpdates: %v", err)
		}
		if len(runningUpds) != 0 {
			t.Fatalf("expected empty Running after dryRun, got %v", runningUpds)
		}
	})

	t.Run("noop target skips targeted Get cleanly", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		f.ds.config.SBI = &config.SBI{Type: "noop"}
		f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		intent := buildJSONIntent(t, f.ds, "noop-intent", 10, false, map[string]any{
			"system": map[string]any{
				"hashed-password": "NoopPassword!",
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-noop-mock", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(noop mock): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-noop-mock"); err != nil {
			t.Fatalf("TransactionConfirm(noop mock): %v", err)
		}

		sc, schemaCfg := initMutateOnWriteSchema(t)
		localCache, err := cache.NewLocalCache(&cacheconfig.CacheConfig{Dir: t.TempDir()})
		if err != nil {
			t.Fatalf("NewLocalCache: %v", err)
		}
		dsCfg := &config.DatastoreConfig{
			Name:       "real-noop-ds",
			Schema:     schemaCfg,
			SBI:        &config.SBI{Type: "noop"},
			Sync:       &config.Sync{},
			Validation: config.NewValidationConfig(),
			Deviation:  &config.DeviationConfig{Interval: time.Hour},
		}
		realNoopDS, err := New(ctx, dsCfg, sc, localCache)
		if err != nil {
			t.Fatalf("datastore.New(noop): %v", err)
		}
		t.Cleanup(func() { _ = realNoopDS.Stop(ctx) })

		deadline := time.Now().Add(2 * time.Second)
		for realNoopDS.ConnectionState().Status != sdcpb.TargetStatus_CONNECTED {
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for noop target connection")
			}
			time.Sleep(5 * time.Millisecond)
		}

		noopTI := buildJSONIntent(t, realNoopDS, "intent1", 10, false, map[string]any{
			"system": map[string]any{
				"hashed-password": "NoopCleartextPassword",
			},
		})
		if _, err := realNoopDS.TransactionSet(ctx, "tx-real-noop", []*types.TransactionIntent{noopTI}, nil, time.Minute, false); err != nil {
			t.Fatalf("realNoopDS.TransactionSet: %v", err)
		}
		if err := realNoopDS.TransactionConfirm(ctx, "tx-real-noop"); err != nil {
			t.Fatalf("realNoopDS.TransactionConfirm: %v", err)
		}
	})

	t.Run("empty readback is not bound as confirmed applied value", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		setCall := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			After(setCall).
			Return(&sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("system", nil),
								sdcpb.NewPathElem("hashed-password", nil),
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: ""}},
					}},
				}},
			}, nil).Times(1)

		intent := buildJSONIntent(t, f.ds, "empty-rb-intent", 10, true, map[string]any{
			"system": map[string]any{
				"hashed-password": "SecretValue",
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-empty-rb", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-empty-rb): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-empty-rb"); err != nil {
			t.Fatalf("TransactionConfirm(tx-empty-rb): %v", err)
		}

		// Empty readback is ignored, so an empty string in Running raises NOT_APPLIED.
		if err := f.ds.ApplyToRunning(ctx, []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": ""},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning: %v", err)
		}
		events := collectWatchDeviationsBatch(t, f.ds)
		foundNotApplied := false
		for _, ev := range events {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				foundNotApplied = true
			}
		}
		if !foundNotApplied {
			t.Fatalf("expected NOT_APPLIED when readback and Running were empty string, got %v", events)
		}
	})
}
