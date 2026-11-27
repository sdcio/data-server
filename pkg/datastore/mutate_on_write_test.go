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
	"github.com/sdcio/sdc-protos/tree_persist"
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
      leaf secret {
        type string {
          length "4..32";
        }
      }
    }
    choice auth-mode {
      case primary {
        leaf primary-auth {
          type ianach:crypt-hash;
        }
      }
      case fallback {
        leaf fallback-auth {
          type ianach:crypt-hash;
        }
      }
    }
  }
  container extra {
    leaf token {
      type ianach:crypt-hash;
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
	return buildJSONIntentWithOpts(t, ds, name, prio, nonRevertive, false, payload)
}

func buildPreviouslyAppliedJSONIntent(t *testing.T, ds *Datastore, name string, prio int32, nonRevertive bool, payload map[string]any) *types.TransactionIntent {
	t.Helper()
	return buildJSONIntentWithOpts(t, ds, name, prio, nonRevertive, true, payload)
}

func buildJSONIntentWithOpts(t *testing.T, ds *Datastore, name string, prio int32, nonRevertive bool, previouslyApplied bool, payload map[string]any) *types.TransactionIntent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ti, err := ds.SdcpbTransactionIntentToInternalTI(context.Background(), &sdcpb.TransactionIntent{
		Intent:            name,
		Priority:          prio,
		NonRevertive:      nonRevertive,
		PreviouslyApplied: previouslyApplied,
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

func hashedPasswordPath() *sdcpb.Path {
	return &sdcpb.Path{Elem: []*sdcpb.PathElem{
		sdcpb.NewPathElem("system", nil),
		sdcpb.NewPathElem("hashed-password", nil),
	}}
}

func expectHashedPasswordSetAndReadback(
	t *testing.T,
	f *mowDatastoreFixture,
	wantCleartext string,
	readback string,
	onReadback func(),
) {
	t.Helper()
	setCall := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			upds, err := source.ToProtoUpdates(setCtx, true)
			if err != nil {
				return nil, err
			}
			if got := updatesByXPath(upds)["/system/hashed-password"]; got != wantCleartext {
				t.Fatalf("southbound value = %q, want %q", got, wantCleartext)
			}
			return &sdcpb.SetDataResponse{}, nil
		})
	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(setCall).DoAndReturn(
		func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
			if len(req.GetPath()) != 1 || req.GetPath()[0].ToXPath(false) != "/system/hashed-password" {
				t.Fatalf("targeted Get paths = %v, want /system/hashed-password", req.GetPath())
			}
			if onReadback != nil {
				onReadback()
			}
			return &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{{
						Path:  hashedPasswordPath(),
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: readback}},
					}},
				}},
			}, nil
		})
}

func assertRunningHashedPassword(t *testing.T, ds *Datastore, want string) {
	t.Helper()
	runningResp, err := ds.GetIntent(context.Background(), consts.RunningIntentName)
	if err != nil {
		t.Fatalf("GetIntent(running): %v", err)
	}
	runningUpds, err := runningResp.ToProtoUpdates(context.Background())
	if err != nil {
		t.Fatalf("runningResp.ToProtoUpdates: %v", err)
	}
	if got := updatesByXPath(runningUpds)["/system/hashed-password"]; got != want {
		t.Fatalf("Running hashed-password = %q, want %q", got, want)
	}
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
			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(syncPayload, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
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

func TestDatastore_MutateOnWrite_RevertiveDriftRecapturesConfirmedValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		cleartext = "MyS3cret!"
		hashV1    = "$6$salt1$hash1"
		driftHash = "$6$other$hash"
		hashV2    = "$6$salt2$hash2"
	)

	expectHashedPasswordSetAndReadback(t, f, cleartext, hashV1, nil)

	intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
		"system": map[string]any{"hashed-password": cleartext},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet: %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
		t.Fatalf("TransactionConfirm: %v", err)
	}

	expectHashedPasswordSetAndReadback(t, f, cleartext, hashV2, func() {
		_, lockErr := f.ds.TransactionSet(context.Background(), "concurrent-sync-readback", nil, nil, time.Minute, false)
		if !errors.Is(lockErr, ErrDatastoreLocked) {
			t.Errorf("concurrent TransactionSet during sync readback returned %v, want ErrDatastoreLocked", lockErr)
		}
	})

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(map[string]any{
		"system": map[string]any{"hashed-password": driftHash},
	}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(drift): %v", err)
	}

	assertRunningHashedPassword(t, f.ds, hashV2)

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(map[string]any{
		"system": map[string]any{"hashed-password": hashV2},
	}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(converged): %v", err)
	}
}

func TestDatastore_MutateOnWrite_NonRevertiveValueDriftReportsNotApplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		cleartext = "MyS3cret!"
		hashV1    = "$6$salt1$hash1"
		driftHash = "$6$other$hash"
	)
	expectHashedPasswordSetAndReadback(t, f, cleartext, hashV1, nil)

	intent := buildJSONIntent(t, f.ds, "intent1", 10, true, map[string]any{
		"system": map[string]any{"hashed-password": cleartext},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet: %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
		t.Fatalf("TransactionConfirm: %v", err)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": driftHash},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(drift): %v", err)
	}

	events := collectWatchDeviationsBatch(t, f.ds)
	for _, ev := range events {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
			ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
			ev.GetIntent() == "intent1" &&
			ev.GetPath().ToXPath(false) == "/system/hashed-password" {
			if ev.GetExpectedValue().GetStringVal() != cleartext || ev.GetCurrentValue().GetStringVal() != driftHash {
				t.Fatalf("NOT_APPLIED expected/current = (%v, %v), want (%q, %q)",
					ev.GetExpectedValue(), ev.GetCurrentValue(), cleartext, driftHash)
			}
			return
		}
	}
	t.Fatalf("expected NOT_APPLIED for mutate-on-write value drift, got %v", events)
}

func TestDatastore_MutateOnWrite_DeletedLeafDrift(t *testing.T) {
	for _, tc := range []struct {
		name         string
		nonRevertive bool
	}{
		{name: "revertive intent restores and recaptures", nonRevertive: false},
		{name: "non-revertive intent reports not applied", nonRevertive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const (
				cleartext = "MyS3cret!"
				hashV1    = "$6$salt1$hash1"
				hashV2    = "$6$salt2$hash2"
			)
			expectHashedPasswordSetAndReadback(t, f, cleartext, hashV1, nil)

			intent := buildJSONIntent(t, f.ds, "intent1", 10, tc.nonRevertive, map[string]any{
				"system": map[string]any{"hashed-password": cleartext},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet: %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
				t.Fatalf("TransactionConfirm: %v", err)
			}

			if !tc.nonRevertive {
				expectHashedPasswordSetAndReadback(t, f, cleartext, hashV2, nil)
			}

			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hostname": "still-present"},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(delete): %v", err)
			}

			if tc.nonRevertive {
				events := collectWatchDeviationsBatch(t, f.ds)
				found := false
				for _, ev := range events {
					if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
						ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
						ev.GetIntent() == "intent1" &&
						ev.GetPath().ToXPath(false) == "/system/hashed-password" {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected NOT_APPLIED for deleted mutate-on-write leaf, got %v", events)
				}
				return
			}

			assertRunningHashedPassword(t, f.ds, hashV2)
			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hashed-password": hashV2},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(restored deletion converged): %v", err)
			}
		})
	}
}

func TestDatastore_MutateOnWrite_PriorityTakeoverAndDeletionPromotion(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		lowCleartext  = "LowSecret!"
		highCleartext = "HighSecret!"
		hashV1        = "$6$low-epoch1$hash"
		hashV2        = "$6$high-epoch$hash"
		hashV3        = "$6$low-epoch2$hash"
	)
	expectHashedPasswordSetAndReadback(t, f, lowCleartext, hashV1, nil)
	lowIntent := buildJSONIntent(t, f.ds, "low-owner", 20, false, map[string]any{
		"system": map[string]any{"hashed-password": lowCleartext},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-low", []*types.TransactionIntent{lowIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(low): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-low"); err != nil {
		t.Fatalf("TransactionConfirm(low): %v", err)
	}

	expectHashedPasswordSetAndReadback(t, f, highCleartext, hashV2, nil)
	highIntent := buildJSONIntent(t, f.ds, "high-owner", 5, false, map[string]any{
		"system": map[string]any{"hashed-password": highCleartext},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-high", []*types.TransactionIntent{highIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(high): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-high"); err != nil {
		t.Fatalf("TransactionConfirm(high): %v", err)
	}
	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": hashV2},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(high owner converged): %v", err)
	}

	deleteHigh, err := f.ds.SdcpbTransactionIntentToInternalTI(ctx, &sdcpb.TransactionIntent{
		Intent:   "high-owner",
		Priority: 5,
		Delete:   true,
	})
	if err != nil {
		t.Fatalf("build high-owner deletion: %v", err)
	}
	expectHashedPasswordSetAndReadback(t, f, lowCleartext, hashV3, nil)
	if _, err := f.ds.TransactionSet(ctx, "tx-delete-high", []*types.TransactionIntent{deleteHigh}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(delete high): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-delete-high"); err != nil {
		t.Fatalf("TransactionConfirm(delete high): %v", err)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": hashV3},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(promoted owner converged): %v", err)
	}
}

func TestDatastore_MutateOnWrite_UnconfirmedTransactionRollbackRecapturesPriorOwner(t *testing.T) {
	for _, tc := range []struct {
		name string
		undo func(t *testing.T, f *mowDatastoreFixture, rollbackDone <-chan struct{})
	}{
		{
			name: "TransactionCancel",
			undo: func(t *testing.T, f *mowDatastoreFixture, _ <-chan struct{}) {
				t.Helper()
				if err := f.ds.TransactionCancel(context.Background(), "tx-change"); err != nil {
					t.Fatalf("TransactionCancel: %v", err)
				}
			},
		},
		{
			name: "rollback timer expiry",
			undo: func(t *testing.T, f *mowDatastoreFixture, rollbackDone <-chan struct{}) {
				t.Helper()
				select {
				case <-rollbackDone:
				case <-time.After(3 * time.Second):
					t.Fatal("timed out waiting for automatic rollback")
				}
				if err := f.ds.TransactionConfirm(context.Background(), "tx-change"); err == nil ||
					!strings.Contains(err.Error(), "no active transaction") {
					t.Fatalf("TransactionConfirm after timer rollback = %v, want no active transaction", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const (
				oldCleartext = "OldSecret!"
				newCleartext = "NewSecret!"
				hashV1       = "$6$old-epoch1$hash"
				hashV2       = "$6$new-epoch$hash"
				hashV3       = "$6$old-epoch2$hash"
			)
			expectHashedPasswordSetAndReadback(t, f, oldCleartext, hashV1, nil)
			oldIntent := buildJSONIntent(t, f.ds, "owner", 10, false, map[string]any{
				"system": map[string]any{"hashed-password": oldCleartext},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-initial", []*types.TransactionIntent{oldIntent}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet(initial): %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-initial"); err != nil {
				t.Fatalf("TransactionConfirm(initial): %v", err)
			}

			expectHashedPasswordSetAndReadback(t, f, newCleartext, hashV2, nil)
			changedIntent := buildJSONIntent(t, f.ds, "owner", 10, false, map[string]any{
				"system": map[string]any{"hashed-password": newCleartext},
			})
			timeout := time.Minute
			if tc.name == "rollback timer expiry" {
				timeout = 100 * time.Millisecond
			}
			if _, err := f.ds.TransactionSet(ctx, "tx-change", []*types.TransactionIntent{changedIntent}, nil, timeout, false); err != nil {
				t.Fatalf("TransactionSet(change): %v", err)
			}

			rollbackDone := make(chan struct{})
			expectHashedPasswordSetAndReadback(t, f, oldCleartext, hashV3, func() { close(rollbackDone) })
			tc.undo(t, f, rollbackDone)

			assertRunningHashedPassword(t, f.ds, hashV3)

			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hashed-password": hashV3},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(rollback converged): %v", err)
			}
		})
	}
}

func TestDatastore_MutateOnWrite_TransactionCancelRollsBackNewLeafToAbsence(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		cleartext = "NewSecret!"
		hash      = "$6$new$hash"
	)
	expectHashedPasswordSetAndReadback(t, f, cleartext, hash, nil)
	intent := buildJSONIntent(t, f.ds, "owner", 10, false, map[string]any{
		"system": map[string]any{"hashed-password": cleartext},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-new", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(new leaf): %v", err)
	}

	f.sbi.EXPECT().
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
			if len(upds) != 0 || len(dels) != 1 {
				t.Fatalf("rollback diff = updates %v, deletes %v; want one deletion and no updates", upds, dels)
			}
			deletePath := dels[0].ToXPath(false)
			if deletePath != "/system" && deletePath != "/system/hashed-password" {
				t.Fatalf("rollback delete path = %q, want hashed-password or its now-empty parent", deletePath)
			}
			return &sdcpb.SetDataResponse{}, nil
		})

	if err := f.ds.TransactionCancel(ctx, "tx-new"); err != nil {
		t.Fatalf("TransactionCancel: %v", err)
	}
	assertRunningHashedPassword(t, f.ds, "")
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
	midNonRevertiveIntent := buildJSONIntent(t, f.ds, "mid-nr-owner", 15, true, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "alice", "password": "HighSecretV2"},
			},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-2", []*types.TransactionIntent{highIntentV2, midNonRevertiveIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-2): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-2"); err != nil {
		t.Fatalf("TransactionConfirm(tx-2): %v", err)
	}

	// Demoting high-owner below mid-nr-owner promotes an already-persisted non-revertive
	// owner with the identical intended value without pushing, proving baseline owner specificity.
	demotedHighIntent := buildJSONIntent(t, f.ds, "high-owner", 25, false, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "alice", "password": "HighSecretV2"},
			},
		},
	})
	expectEmptySet(t, f.sbi)
	if _, err := f.ds.TransactionSet(ctx, "tx-3", []*types.TransactionIntent{demotedHighIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-3): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-3"); err != nil {
		t.Fatalf("TransactionConfirm(tx-3): %v", err)
	}

	eventsAfterOwnerChange := collectWatchDeviationsBatch(t, f.ds)
	foundNotAppliedForPromoted := false
	for _, ev := range eventsAfterOwnerChange {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
			ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
			ev.GetIntent() == "mid-nr-owner" &&
			ev.GetPath().ToXPath(false) == "/system/user[username=alice]/password" {
			foundNotAppliedForPromoted = true
			if ev.GetExpectedValue().GetStringVal() != "HighSecretV2" {
				t.Fatalf("NOT_APPLIED ExpectedValue = %v, want HighSecretV2", ev.GetExpectedValue())
			}
		}
	}
	if !foundNotAppliedForPromoted {
		t.Fatalf("expected NOT_APPLIED for mid-nr-owner because baseline belonged to high-owner, got %v", eventsAfterOwnerChange)
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
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, jsonImporter.NewJsonTreeImporter(map[string]any{
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

func TestDatastore_MutateOnWrite_FailedReadbackUsesCausalPassiveSync(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readback func() (*sdcpb.GetDataResponse, error)
	}{
		{
			name: "targeted Get error",
			readback: func() (*sdcpb.GetDataResponse, error) {
				return nil, errors.New("targeted readback unavailable")
			},
		},
		{
			name: "targeted Get omits requested path",
			readback: func() (*sdcpb.GetDataResponse, error) {
				return &sdcpb.GetDataResponse{}, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const (
				cleartext = "PendingSecret!"
				staleHash = "$6$before-set$hash"
				freshHash = "$6$after-set$hash"
			)

			slowSyncStartedAt := time.Now().UnixNano()
			setCall := f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				Return(&sdcpb.SetDataResponse{}, nil).
				Times(1)
			f.sbi.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				After(setCall).
				DoAndReturn(func(context.Context, *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
					return tc.readback()
				}).
				Times(1)

			intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
				"system": map[string]any{"hashed-password": cleartext},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-pending", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet: %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-pending"); err != nil {
				t.Fatalf("TransactionConfirm: %v", err)
			}

			assertRunningHashedPassword(t, f.ds, cleartext)

			if err := f.ds.ApplyToRunning(ctx, slowSyncStartedAt, []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hashed-password": staleHash},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(pre-Set slow sync): %v", err)
			}
			assertRunningHashedPassword(t, f.ds, cleartext)

			causalSyncStartedAt := time.Now().UnixNano()
			if err := f.ds.ApplyToRunning(ctx, causalSyncStartedAt, []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hashed-password": freshHash},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(post-Set sync): %v", err)
			}
			assertRunningHashedPassword(t, f.ds, freshHash)

			if err := f.ds.ApplyToRunning(ctx, slowSyncStartedAt, []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hashed-password": staleHash},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(late pre-Set slow sync): %v", err)
			}
			assertRunningHashedPassword(t, f.ds, freshHash)

			for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
					ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
					ev.GetPath().ToXPath(false) == "/system/hashed-password" {
					t.Fatalf("unexpected NOT_APPLIED after passive capture: %v", ev)
				}
			}
		})
	}
}

func TestDatastore_MutateOnWrite_PendingCaptureClearsWhenCausalSyncOmitsLeaf(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	setCall := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)
	f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(setCall).
		Return(nil, errors.New("targeted readback unavailable")).
		Times(1)

	intent := buildJSONIntent(t, f.ds, "intent1", 10, true, map[string]any{
		"system": map[string]any{"hashed-password": "MissingSecret!"},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-missing", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet: %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-missing"); err != nil {
		t.Fatalf("TransactionConfirm: %v", err)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hostname": "still-present"},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(causal missing leaf): %v", err)
	}

	for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
			ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
			ev.GetIntent() == "intent1" &&
			ev.GetPath().ToXPath(false) == "/system/hashed-password" {
			return
		}
	}
	t.Fatal("expected NOT_APPLIED after a causal sync omitted the pending mutate-on-write leaf")
}

func TestDatastore_MutateOnWrite_UnannotatedStringCryptDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		targetVal string
	}{
		{name: "md5 $1$", targetVal: "$1$saltsalt$abcdefghijklmnopqrstuv"},
		{name: "bcrypt $2a$", targetVal: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{name: "bcrypt $2b$", targetVal: "$2b$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{name: "bcrypt $2x$", targetVal: "$2x$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{name: "bcrypt $2y$", targetVal: "$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{name: "sha256 $5$", targetVal: "$5$rounds=5000$saltsalt$hashed256value"},
		{name: "sha512 $6$", targetVal: "$6$rounds=5000$saltsalt$hashed512value"},
		{name: "scrypt mcf $7$", targetVal: "$7$CU..../....$salt$hashedvalue"},
		{name: "type8 $8$", targetVal: "$8$dsYGNam3K1SIJO$7nv/35M2qr6t.dVc7UY9zrX"},
		{name: "type9/junos $9$", targetVal: "$9$1k2j3h4g5f6d7s8a"},
		{name: "yescrypt $y$", targetVal: "$y$j9T$salt$hashedvalue"},
		{name: "gost-yescrypt $gy$", targetVal: "$gy$j9T$salt$hashedvalue"},
		{name: "ar2 $ar2$", targetVal: "$ar2$v=19$salt$hashedvalue"},
		{name: "argon2i $argon2i$", targetVal: "$argon2i$v=19$m=4096,t=3,p=1$c2FsdA$aGFzaA"},
		{name: "argon2id $argon2id$", targetVal: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"},
		{name: "pbkdf2 $pbkdf2$", targetVal: "$pbkdf2$1000$c2FsdA$aGFzaA"},
		{name: "pbkdf2-sha256 $pbkdf2-sha256$", targetVal: "$pbkdf2-sha256$29000$c2FsdA$aGFzaA"},
		{name: "scrypt $scrypt$", targetVal: "$scrypt$ln=16,r=8,p=1$c2FsdA$aGFzaA"},
		{name: "sha1 $sha1$", targetVal: "$sha1$40000$saltsalt$hashedsha1value"},
		{name: "passphrase prefix 000PASSPHRASE000", targetVal: "000PASSPHRASE000dGVzdGNpcGhlcnRleHQ="},
		{name: "passphrase sentinel 000PASSPHRASE000", targetVal: "000PASSPHRASE000"},
		{name: "sros token hash", targetVal: "dGhpcyBpcyBhIHRlc3Q= hash"},
		{name: "sros token hash2", targetVal: "dGhpcyBpcyBhIHRlc3Q= hash2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const cleartext = "KeychainSecret1"

			slowSyncStartedAt := time.Now().UnixNano()
			f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				Return(&sdcpb.SetDataResponse{}, nil).
				Times(1)
			f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

			intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
				"system": map[string]any{
					"user": []any{map[string]any{
						"username": "alice",
						"secret":   cleartext,
					}},
				},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-discover", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet: %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-discover"); err != nil {
				t.Fatalf("TransactionConfirm: %v", err)
			}

			// Slow pre-Set sync must not revert or bind stale data.
			if err := f.ds.ApplyToRunning(ctx, slowSyncStartedAt, []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{
						"user": []any{map[string]any{
							"username": "alice",
							"secret":   "$6$stale$hash",
						}},
					},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(pre-Set slow sync): %v", err)
			}

			// First causal sync binds Running's crypt/cipher format with zero revert Sets.
			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{
						"user": []any{map[string]any{
							"username": "alice",
							"secret":   tc.targetVal,
						}},
					},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(causal sync): %v", err)
			}

			intentResp, err := f.ds.GetIntent(ctx, "intent1")
			if err != nil {
				t.Fatalf("GetIntent(intent1): %v", err)
			}
			intentUpds, err := intentResp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("intentResp.ToProtoUpdates: %v", err)
			}
			if got := updatesByXPath(intentUpds)["/system/user[username=alice]/secret"]; got != cleartext {
				t.Fatalf("GetIntent(intent1) secret = %q, want %q", got, cleartext)
			}

			runningResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
			if err != nil {
				t.Fatalf("GetIntent(running): %v", err)
			}
			runningUpds, err := runningResp.ToProtoUpdates(ctx)
			if err != nil {
				t.Fatalf("runningResp.ToProtoUpdates: %v", err)
			}
			if got := updatesByXPath(runningUpds)["/system/user[username=alice]/secret"]; got != tc.targetVal {
				t.Fatalf("GetIntent(running) secret = %q, want %q", got, tc.targetVal)
			}

			for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
					ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
					t.Fatalf("unexpected NOT_APPLIED deviation after crypt discovery: %v", ev)
				}
			}

			blame, err := f.ds.BlameConfig(ctx, false)
			if err != nil {
				t.Fatalf("BlameConfig: %v", err)
			}
			secretBlame := testhelper.FindBlameElement(blame, "system", "user", "alice", "secret")
			if secretBlame == nil {
				t.Fatalf("missing secret blame node in %v", blame)
			}
			if secretBlame.GetValue().GetStringVal() != cleartext || secretBlame.GetDeviationValue() != nil {
				t.Fatalf("BlameConfig secret = (value=%v, deviation=%v), want (%q, nil)",
					secretBlame.GetValue(), secretBlame.GetDeviationValue(), cleartext)
			}
		})
	}
}

func TestDatastore_MutateOnWrite_LearnedKeylessSchemaPathPromotesSubsequentWritesToTargetedGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		aliceCleartext = "AliceSecret1"
		aliceHash      = "$6$rounds=5000$alicesalt$longhashedvalueexceeding32characterslimit"
		bobCleartext   = "BobSecret2"
		bobHash        = "$6$rounds=5000$bobsalt$longbobhashedvalueexceeding32characterslim"
		replaceHash    = "$6$rounds=5000$replsalt$longreplacehashedvalueexceeding32charlim"
	)

	// 1. First write on alice: unannotated plain string leaf -> Set only, no targeted Get.
	f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)

	aliceIntent := buildJSONIntent(t, f.ds, "alice-intent", 10, false, map[string]any{
		"system": map[string]any{
			"user": []any{map[string]any{
				"username": "alice",
				"secret":   aliceCleartext,
			}},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-alice", []*types.TransactionIntent{aliceIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-alice): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-alice"); err != nil {
		t.Fatalf("TransactionConfirm(tx-alice): %v", err)
	}

	// 2. First causal sync discovers crypt prefix on alice and learns /system/user/secret.
	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   aliceHash,
				}},
			},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(alice causal sync): %v", err)
	}

	// 3. Subsequent write on another instance (bob) of the learned keyless path
	// issues targeted post-Set Get immediately and does not re-push converged alice.
	bobSet := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			upds, err := source.ToProtoUpdates(setCtx, true)
			if err != nil {
				return nil, err
			}
			pushed := updatesByXPath(upds)
			if _, repushedAlice := pushed["/system/user[username=alice]/secret"]; repushedAlice {
				t.Fatalf("converged alice secret was unexpectedly re-pushed: %v", pushed)
			}
			if got := pushed["/system/user[username=bob]/secret"]; got != bobCleartext {
				t.Fatalf("pushed bob secret = %q, want %q", got, bobCleartext)
			}
			return &sdcpb.SetDataResponse{}, nil
		}).
		Times(1)

	f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(bobSet).
		DoAndReturn(func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
			if len(req.GetPath()) != 1 || req.GetPath()[0].ToXPath(false) != "/system/user[username=bob]/secret" {
				t.Fatalf("targeted Get paths = %v, want [/system/user[username=bob]/secret]", req.GetPath())
			}
			return &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("system", nil),
								sdcpb.NewPathElem("user", map[string]string{"username": "bob"}),
								sdcpb.NewPathElem("secret", nil),
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: bobHash}},
					}},
				}},
			}, nil
		}).
		Times(1)

	aliceRebundled := buildJSONIntent(t, f.ds, "alice-intent", 10, false, map[string]any{
		"system": map[string]any{
			"user": []any{map[string]any{
				"username": "alice",
				"secret":   aliceCleartext,
			}},
		},
	})
	bobIntent := buildJSONIntent(t, f.ds, "bob-intent", 10, false, map[string]any{
		"system": map[string]any{
			"user": []any{map[string]any{
				"username": "bob",
				"secret":   bobCleartext,
			}},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-bob", []*types.TransactionIntent{aliceRebundled, bobIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-bob): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-bob"); err != nil {
		t.Fatalf("TransactionConfirm(tx-bob): %v", err)
	}

	runningResp, err := f.ds.GetIntent(ctx, consts.RunningIntentName)
	if err != nil {
		t.Fatalf("GetIntent(running): %v", err)
	}
	runningUpds, err := runningResp.ToProtoUpdates(ctx)
	if err != nil {
		t.Fatalf("runningResp.ToProtoUpdates: %v", err)
	}
	if got := updatesByXPath(runningUpds)["/system/user[username=bob]/secret"]; got != bobHash {
		t.Fatalf("GetIntent(running) bob secret = %q, want immediate targeted readback %q", got, bobHash)
	}

	// 4. Orphan-delete alice-intent while modifying hostname: Running retains alice's >32-char hash
	// and validation skips Running on the learned mutate-on-write path just like schema-flagged leaves.
	f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)

	orphanAlice, err := f.ds.SdcpbTransactionIntentToInternalTI(ctx, &sdcpb.TransactionIntent{
		Intent:   "alice-intent",
		Priority: 10,
		Delete:   true,
		Orphan:   true,
	})
	if err != nil {
		t.Fatalf("SdcpbTransactionIntentToInternalTI(orphanAlice): %v", err)
	}
	hostIntent := buildJSONIntent(t, f.ds, "host-intent", 10, false, map[string]any{
		"system": map[string]any{"hostname": "router-1"},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-orphan-alice", []*types.TransactionIntent{orphanAlice, hostIntent}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-orphan-alice): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-orphan-alice"); err != nil {
		t.Fatalf("TransactionConfirm(tx-orphan-alice): %v", err)
	}

	// 5. Full-config replace matching persisted intents preserves the learned path,
	// issues targeted Get immediately, and binds the new baseline without a second Set.
	replSet := f.sbi.EXPECT().
		Set(gomock.Any(), gomock.Any()).
		Return(&sdcpb.SetDataResponse{}, nil).
		Times(1)
	replGet := f.sbi.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		After(replSet).
		DoAndReturn(func(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
			if len(req.GetPath()) != 1 || req.GetPath()[0].ToXPath(false) != "/system/user[username=bob]/secret" {
				t.Fatalf("replace targeted Get paths = %v, want [/system/user[username=bob]/secret]", req.GetPath())
			}
			return &sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("system", nil),
								sdcpb.NewPathElem("user", map[string]string{"username": "bob"}),
								sdcpb.NewPathElem("secret", nil),
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: replaceHash}},
					}},
				}},
			}, nil
		}).
		Times(1)
	expectEmptySet(t, f.sbi).After(replGet)

	replaceTI := buildJSONIntent(t, f.ds, consts.ReplaceIntentName, consts.ReplaceValuesPrio, false, map[string]any{
		"system": map[string]any{
			"hostname": "router-1",
			"user": []any{map[string]any{
				"username": "bob",
				"secret":   bobCleartext,
			}},
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-replace-learned", nil, replaceTI, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-replace-learned): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-replace-learned"); err != nil {
		t.Fatalf("TransactionConfirm(tx-replace-learned): %v", err)
	}
}

func TestDatastore_MutateOnWrite_PreHashedIntentEnforcesByteForByteDrift(t *testing.T) {
	const (
		preHashedA = "$6$saltA$hashA"
		preHashedB = "$6$saltB$hashB"
	)

	t.Run("unannotated plain string pre-hashed intent reverts and does not learn path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		ctx := context.Background()

		txSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
			"system": map[string]any{
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   preHashedA,
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-prehashed", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet: %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-prehashed"); err != nil {
			t.Fatalf("TransactionConfirm: %v", err)
		}

		// Differing hash in Running must trigger southbound revert pushing preHashedA.
		revertSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			After(txSet).
			DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
				upds, err := source.ToProtoUpdates(setCtx, true)
				if err != nil {
					return nil, err
				}
				if got := updatesByXPath(upds)["/system/user[username=alice]/secret"]; got != preHashedA {
					t.Fatalf("revert pushed secret = %q, want %q", got, preHashedA)
				}
				return &sdcpb.SetDataResponse{}, nil
			}).
			Times(1)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{map[string]any{
						"username": "alice",
						"secret":   preHashedB,
					}},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(differing hash): %v", err)
		}

		foundNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/user[username=alice]/secret" {
				foundNotApplied = true
			}
		}
		if !foundNotApplied {
			t.Fatal("expected NOT_APPLIED deviation when Running holds differing hash for pre-hashed intent")
		}

		// Subsequent write must still not issue targeted Get because path was not learned.
		f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			After(revertSet).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)

		bobIntent := buildJSONIntent(t, f.ds, "bob-intent", 10, false, map[string]any{
			"system": map[string]any{
				"user": []any{map[string]any{
					"username": "bob",
					"secret":   "BobCleartext",
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-bob-after-prehashed", []*types.TransactionIntent{bobIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-bob-after-prehashed): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-bob-after-prehashed"); err != nil {
			t.Fatalf("TransactionConfirm(tx-bob-after-prehashed): %v", err)
		}
	})

	t.Run("learned path pre-hashed intent rejects differing readback and sync hashes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		ctx := context.Background()

		// First learn /system/user/secret via cleartext alice.
		f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)
		aliceIntent := buildJSONIntent(t, f.ds, "alice-intent", 10, false, map[string]any{
			"system": map[string]any{
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   "AliceCleartext",
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-learn-alice", []*types.TransactionIntent{aliceIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-learn-alice): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-learn-alice"); err != nil {
			t.Fatalf("TransactionConfirm(tx-learn-alice): %v", err)
		}
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{map[string]any{
						"username": "alice",
						"secret":   "$6$alice$learned",
					}},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(learn alice): %v", err)
		}

		// Now author a pre-hashed intent on bob (same learned keyless path /system/user/secret),
		// where targeted Get fails and causal sync returns differing preHashedB.
		bobSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)
		bobGet := f.sbi.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			After(bobSet).
			Return(nil, errors.New("readback unavailable")).
			Times(1)

		bobIntent := buildJSONIntent(t, f.ds, "bob-intent", 10, false, map[string]any{
			"system": map[string]any{
				"user": []any{map[string]any{
					"username": "bob",
					"secret":   preHashedA,
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-prehashed-bob", []*types.TransactionIntent{bobIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-prehashed-bob): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-prehashed-bob"); err != nil {
			t.Fatalf("TransactionConfirm(tx-prehashed-bob): %v", err)
		}

		// Causal sync with differing preHashedB must revert bob to preHashedA (and targeted Get on revert).
		revSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			After(bobGet).
			DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
				upds, err := source.ToProtoUpdates(setCtx, true)
				if err != nil {
					return nil, err
				}
				if got := updatesByXPath(upds)["/system/user[username=bob]/secret"]; got != preHashedA {
					t.Fatalf("revert pushed bob secret = %q, want %q", got, preHashedA)
				}
				return &sdcpb.SetDataResponse{}, nil
			}).
			Times(1)
		f.sbi.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			After(revSet).
			Return(&sdcpb.GetDataResponse{
				Notification: []*sdcpb.Notification{{
					Update: []*sdcpb.Update{{
						Path: &sdcpb.Path{
							IsRootBased: true,
							Elem: []*sdcpb.PathElem{
								sdcpb.NewPathElem("system", nil),
								sdcpb.NewPathElem("user", map[string]string{"username": "bob"}),
								sdcpb.NewPathElem("secret", nil),
							},
						},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: preHashedB}},
					}},
				}},
			}, nil).
			Times(1)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{
						map[string]any{"username": "alice", "secret": "$6$alice$learned"},
						map[string]any{"username": "bob", "secret": preHashedB},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(bob differing hash): %v", err)
		}

		// Even when targeted Get returned preHashedB on the revert, preHashedB must NOT be
		// bound as a confirmed baseline for preHashedA — NOT_APPLIED must still be raised.
		foundBobNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/user[username=bob]/secret" {
				foundBobNotApplied = true
			}
		}
		if !foundBobNotApplied {
			t.Fatal("expected NOT_APPLIED when targeted Get returned differing hash for pre-hashed intent")
		}
	})
}

func TestDatastore_MutateOnWrite_UnannotatedNonCryptDriftRevertsWithoutLearning(t *testing.T) {
	for _, driftedVal := range []string{
		"drifted-plain",
		"$0$cleartext",
		"$3$nthash",
		"$4$unknown",
		"$argon2d$v=19$m=4096$salt$hash",
		"$6$",
		"$pbkdf2$",
		"two tokens hash",
		" hash",
		"hash2",
	} {
		t.Run("revertive rejects_"+driftedVal, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := newMOWDatastoreFixture(t, ctrl)
			ctx := context.Background()

			const authored = "router-1"

			txSet := f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				Return(&sdcpb.SetDataResponse{}, nil).
				Times(1)
			f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

			intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
				"system": map[string]any{"hostname": authored},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-host", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet: %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-host"); err != nil {
				t.Fatalf("TransactionConfirm: %v", err)
			}

			revertSet := f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				After(txSet).
				DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
					upds, err := source.ToProtoUpdates(setCtx, true)
					if err != nil {
						return nil, err
					}
					if got := updatesByXPath(upds)["/system/hostname"]; got != authored {
						t.Fatalf("revert pushed hostname = %q, want %q", got, authored)
					}
					return &sdcpb.SetDataResponse{}, nil
				}).
				Times(1)

			if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hostname": driftedVal},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(non-crypt drift %q): %v", driftedVal, err)
			}

			foundNotApplied := false
			for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
					ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
					ev.GetPath().ToXPath(false) == "/system/hostname" {
					foundNotApplied = true
				}
			}
			if !foundNotApplied {
				t.Fatalf("expected NOT_APPLIED for non-crypt drift %q", driftedVal)
			}

			// Path was not learned, so a subsequent TransactionSet still issues zero targeted Gets.
			f.sbi.EXPECT().
				Set(gomock.Any(), gomock.Any()).
				After(revertSet).
				Return(&sdcpb.SetDataResponse{}, nil).
				Times(1)

			intentV2 := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
				"system": map[string]any{"hostname": "router-2"},
			})
			if _, err := f.ds.TransactionSet(ctx, "tx-host-v2", []*types.TransactionIntent{intentV2}, nil, time.Minute, false); err != nil {
				t.Fatalf("TransactionSet(v2): %v", err)
			}
			if err := f.ds.TransactionConfirm(ctx, "tx-host-v2"); err != nil {
				t.Fatalf("TransactionConfirm(v2): %v", err)
			}
		})
	}

	t.Run("first causal sync clears pending marker so later crypt-shaped CLI drift still reverts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		ctx := context.Background()

		const authored = "router-clean"

		txSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
			"system": map[string]any{"hostname": authored},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-clean", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet: %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-clean"); err != nil {
			t.Fatalf("TransactionConfirm: %v", err)
		}

		// First causal sync echoes authored value -> clears pendingCaptureAt without learning.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hostname": authored},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(clean first sync): %v", err)
		}

		// Later CLI edit to a crypt-shaped value must NOT be accepted as a Set mutation.
		f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			After(txSet).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hostname": "$6$cli-injected$hash"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(later CLI drift): %v", err)
		}
	})

	t.Run("incremental sync with nil deletes clears unchanged pending marker and discovers crypt secrets", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		ctx := context.Background()

		const (
			authoredHost   = "stream-router"
			authoredSecret = "StreamSecret1"
			hashedSecret   = "$6$stream$hashedsecretvalue"
		)

		txSet := f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			Return(&sdcpb.SetDataResponse{}, nil).
			Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		intent := buildJSONIntent(t, f.ds, "intent1", 10, false, map[string]any{
			"system": map[string]any{
				"hostname": authoredHost,
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   authoredSecret,
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-stream", []*types.TransactionIntent{intent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet: %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-stream"); err != nil {
			t.Fatalf("TransactionConfirm: %v", err)
		}

		// Incremental sync (deletes == nil) on an unrelated leaf must NOT clear alice's pending marker.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), nil,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{map[string]any{
						"username": "other",
					}},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(unrelated stream update): %v", err)
		}

		// First causal incremental sync covering hostname (unchanged) and alice secret (hashed)
		// with deletes == nil must bind alice's hash AND clear hostname's pending marker.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), nil,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hostname": authoredHost,
					"user": []any{map[string]any{
						"username": "alice",
						"secret":   hashedSecret,
					}},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(causal stream update): %v", err)
		}

		// Later out-of-band CLI edit of hostname to a crypt-shaped value via incremental sync
		// must revert to authoredHost rather than being auto-accepted or learned.
		f.sbi.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			After(txSet).
			DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
				upds, err := source.ToProtoUpdates(setCtx, true)
				if err != nil {
					return nil, err
				}
				if got := updatesByXPath(upds)["/system/hostname"]; got != authoredHost {
					t.Fatalf("revert pushed hostname = %q, want %q", got, authoredHost)
				}
				return &sdcpb.SetDataResponse{}, nil
			}).
			Times(1)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), nil,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hostname": "$6$later-stream-cli$hash"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(later stream CLI drift): %v", err)
		}
	})

	t.Run("unrelated nil-importer delete and keyless list sync paths handle pending markers accurately", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)
		ctx := context.Background()

		// Seed initial state then rotate alice's unannotated secret so Running has IsUpdated=true.
		f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(2)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		seed := buildJSONIntent(t, f.ds, "intent1", 10, true, map[string]any{
			"system": map[string]any{
				"hostname": "old-host",
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   "SecretV1",
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-seed", []*types.TransactionIntent{seed}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(seed): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-seed"); err != nil {
			t.Fatalf("TransactionConfirm(seed): %v", err)
		}

		rotated := buildJSONIntent(t, f.ds, "intent1", 10, true, map[string]any{
			"system": map[string]any{
				"hostname": "old-host",
				"user": []any{map[string]any{
					"username": "alice",
					"secret":   "SecretV2",
				}},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-rotate", []*types.TransactionIntent{rotated}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(rotate): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-rotate"); err != nil {
			t.Fatalf("TransactionConfirm(rotate): %v", err)
		}

		// Unrelated nil-importer delete on /system/hostname must NOT evict alice's pending marker.
		unrelatedDel := []*sdcpb.Path{{
			IsRootBased: true,
			Elem:        []*sdcpb.PathElem{sdcpb.NewPathElem("system", nil), sdcpb.NewPathElem("hostname", nil)},
		}}
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), unrelatedDel, nil); err != nil {
			t.Fatalf("ApplyToRunning(unrelated nil importer): %v", err)
		}

		// Syncing keyless list path /system/user covers alice and discovers the crypt secret cleanly.
		keylessUserList := []*sdcpb.Path{{
			IsRootBased: true,
			Elem:        []*sdcpb.PathElem{sdcpb.NewPathElem("system", nil), sdcpb.NewPathElem("user", nil)},
		}}
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), keylessUserList,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hostname": "old-host",
					"user": []any{map[string]any{
						"username": "alice",
						"secret":   "$6$rotated$hashV2",
					}},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(keyless list sync): %v", err)
		}

		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				t.Fatalf("unexpected NOT_APPLIED after keyless list crypt discovery: %v", ev)
			}
		}
	})
}

func TestDatastore_MutateOnWrite_PreviouslyAppliedImmediateSeedWhenRunningPresent(t *testing.T) {
	ctx := context.Background()

	t.Run("seeds mutate-on-write and unannotated crypt leaves immediately when Running is present", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		const (
			cleartextPw      = "RecoveredPw1!"
			hashedPw         = "$6$rec1$hashedpassword"
			cleartextOverlay = "RecoveredOv1!"
			hashedOverlay    = "$6$rec2$hashedoverlay"
			cleartextAlice   = "RecoveredAlice1"
			hashedAlice      = "$6$rec3$hashedalice"
			preHashedBobA    = "$6$bobA$hashvalueA"
			preHashedBobB    = "$6$bobB$hashvalueB"
		)

		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hashed-password":  hashedPw,
					"overlay-password": hashedOverlay,
					"hostname":         "drifted-plain-host",
					"user": []any{
						map[string]any{"username": "alice", "secret": hashedAlice},
						map[string]any{"username": "bob", "secret": preHashedBobB},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(initial): %v", err)
		}

		expectEmptySet(t, f.sbi)
		recovered := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-recovered", 10, true, map[string]any{
			"system": map[string]any{
				"hashed-password":  cleartextPw,
				"overlay-password": cleartextOverlay,
				"hostname":         "router-1",
				"user": []any{
					map[string]any{"username": "alice", "secret": cleartextAlice},
					map[string]any{"username": "bob", "secret": preHashedBobA},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-recover", []*types.TransactionIntent{recovered}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-recover): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-recover"); err != nil {
			t.Fatalf("TransactionConfirm(tx-recover): %v", err)
		}

		persistedBlob, err := f.localCache.InstanceIntentGet(ctx, f.ds.Name(), "nr-recovered")
		if err != nil {
			t.Fatalf("InstanceIntentGet(nr-recovered): %v", err)
		}
		rawBlob := persistedBlob.String()
		for _, h := range []string{hashedPw, hashedOverlay, hashedAlice} {
			if strings.Contains(rawBlob, h) {
				t.Fatalf("persisted intent blob leaked seeded hash %q: %s", h, rawBlob)
			}
		}

		notAppliedPaths := map[string]bool{}
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				notAppliedPaths[ev.GetPath().ToXPath(false)] = true
			}
		}
		for _, cleanPath := range []string{
			"/system/hashed-password",
			"/system/overlay-password",
			"/system/user[username=alice]/secret",
		} {
			if notAppliedPaths[cleanPath] {
				t.Fatalf("unexpected NOT_APPLIED on seeded path %s (all NOT_APPLIED: %v)", cleanPath, notAppliedPaths)
			}
		}
		if !notAppliedPaths["/system/hostname"] {
			t.Fatalf("expected NOT_APPLIED on non-crypt drifted plain string /system/hostname, got %v", notAppliedPaths)
		}
		if !notAppliedPaths["/system/user[username=bob]/secret"] {
			t.Fatalf("expected NOT_APPLIED on pre-hashed mismatch /system/user[username=bob]/secret, got %v", notAppliedPaths)
		}

		blame, err := f.ds.BlameConfig(ctx, false)
		if err != nil {
			t.Fatalf("BlameConfig: %v", err)
		}
		for _, leaf := range []string{"hashed-password", "overlay-password"} {
			el := testhelper.FindBlameElement(blame, "system", leaf)
			if el == nil || el.GetDeviationValue() != nil {
				t.Fatalf("BlameConfig %s = %v, want non-nil with nil DeviationValue", leaf, el)
			}
		}
	})

	t.Run("overruled PreviouslyApplied intent does not clobber higher-priority owner baseline", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		expectHashedPasswordSetAndReadback(t, f, "HighCleartext!", "$6$high$hash", nil)
		high := buildJSONIntent(t, f.ds, "high-owner", 5, false, map[string]any{
			"system": map[string]any{"hashed-password": "HighCleartext!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-high", []*types.TransactionIntent{high}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-high): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-high"); err != nil {
			t.Fatalf("TransactionConfirm(tx-high): %v", err)
		}

		expectEmptySet(t, f.sbi)
		overruledNR := buildPreviouslyAppliedJSONIntent(t, f.ds, "low-nr", 20, true, map[string]any{
			"system": map[string]any{"hashed-password": "LowCleartext!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-low-nr", []*types.TransactionIntent{overruledNR}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-low-nr): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-low-nr"); err != nil {
			t.Fatalf("TransactionConfirm(tx-low-nr): %v", err)
		}

		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				t.Fatalf("overruled PreviouslyApplied intent clobbered winning baseline: %v", ev)
			}
		}
	})
}

func TestDatastore_MutateOnWrite_PreviouslyAppliedBeforeInitialSync(t *testing.T) {
	ctx := context.Background()

	t.Run("replaying PreviouslyApplied before slow initial sync finishes seeds present leaves and preserves NOT_APPLIED on absent leaves", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		const (
			cleartextPw      = "SlowSyncPw1!"
			hashedPw         = "$6$slow1$hashedpw"
			cleartextAlice   = "SlowSyncAlice1"
			hashedAlice      = "$6$slow2$hashedalice"
			missingOverlayPw = "MissingOverlay1!"
		)

		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		// Snapshot starts before control-plane recovery replays PreviouslyApplied.
		slowSyncStartedAt := time.Now().UnixNano()
		time.Sleep(time.Millisecond)

		expectEmptySet(t, f.sbi)
		recovered := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-before-sync", 10, true, map[string]any{
			"system": map[string]any{
				"hashed-password":  cleartextPw,
				"overlay-password": missingOverlayPw,
				"hostname":         "router-1",
				"user": []any{
					map[string]any{"username": "alice", "secret": cleartextAlice},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-before-sync", []*types.TransactionIntent{recovered}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-before-sync): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-before-sync"); err != nil {
			t.Fatalf("TransactionConfirm(tx-before-sync): %v", err)
		}

		// Unrelated incremental update before full sync finishes must not clear awaitingInitialSyncSeed.
		if err := f.ds.ApplyToRunning(ctx, slowSyncStartedAt, nil,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{
						map[string]any{"username": "other"},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(unrelated incremental): %v", err)
		}

		// Slow initial sync completes: hashed-password and alice/secret are present, overlay-password is absent.
		if err := f.ds.ApplyToRunning(ctx, slowSyncStartedAt, []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hashed-password": hashedPw,
					"hostname":        "drifted-plain",
					"user": []any{
						map[string]any{"username": "alice", "secret": hashedAlice},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(slow initial sync): %v", err)
		}

		notApplied := map[string]*sdcpb.WatchDeviationResponse{}
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				notApplied[ev.GetPath().ToXPath(false)] = ev
			}
		}
		for _, seededPath := range []string{"/system/hashed-password", "/system/user[username=alice]/secret"} {
			if ev, bad := notApplied[seededPath]; bad {
				t.Fatalf("unexpected NOT_APPLIED on deferred-seeded path %s: %v", seededPath, ev)
			}
		}
		ovEv, hasMissingOverlay := notApplied["/system/overlay-password"]
		if !hasMissingOverlay {
			t.Fatalf("expected NOT_APPLIED on genuinely absent /system/overlay-password, got %v", notApplied)
		}
		if ovEv.GetCurrentValue() != nil || ovEv.GetExpectedValue().GetStringVal() != missingOverlayPw {
			t.Fatalf("absent /system/overlay-password event = %v, want nil CurrentValue and ExpectedValue %q", ovEv, missingOverlayPw)
		}
		if _, hasDriftedHost := notApplied["/system/hostname"]; !hasDriftedHost {
			t.Fatalf("expected NOT_APPLIED on non-crypt /system/hostname, got %v", notApplied)
		}

		// awaitingInitialSyncSeed was cleared on /system/overlay-password when initial sync observed Running == nil,
		// so a later out-of-band CLI password on that path must still raise NOT_APPLIED.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hashed-password":  hashedPw,
					"overlay-password": "$6$later-cli$unexpectedhash",
					"hostname":         "router-1",
					"user": []any{
						map[string]any{"username": "alice", "secret": hashedAlice},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(later CLI overlay-password): %v", err)
		}

		foundOverlayStillNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/overlay-password" {
				foundOverlayStillNotApplied = true
			}
		}
		if !foundOverlayStillNotApplied {
			t.Fatal("expected /system/overlay-password to remain NOT_APPLIED after later CLI edit because awaitingInitialSyncSeed was cleared")
		}
	})

	t.Run("PreviouslyApplied after initial sync already confirmed leaf absent does not arm awaitingInitialSyncSeed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		// Initial sync completes first with hashed-password absent on target.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hostname": "router-1"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(initial empty): %v", err)
		}

		expectEmptySet(t, f.sbi)
		recovered := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-after-empty-sync", 10, true, map[string]any{
			"system": map[string]any{"hashed-password": "ShouldBeMissing1!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-after-empty-sync", []*types.TransactionIntent{recovered}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet: %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-after-empty-sync"); err != nil {
			t.Fatalf("TransactionConfirm: %v", err)
		}

		// Subsequent out-of-band CLI edit must NOT be auto-seeded as initial sync.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hostname":        "router-1",
					"hashed-password": "$6$cli$unrelatedhash",
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(later CLI): %v", err)
		}

		foundNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/hashed-password" {
				foundNotApplied = true
			}
		}
		if !foundNotApplied {
			t.Fatal("expected NOT_APPLIED when leaf was already absent on initial sync and later edited via CLI")
		}
	})
}

type writeCountingCacheClient struct {
	cache.CacheClientBound
	modifyCalls int
	deleteCalls int
}

func (w *writeCountingCacheClient) IntentModify(ctx context.Context, intent *tree_persist.Intent) error {
	w.modifyCalls++
	return w.CacheClientBound.IntentModify(ctx, intent)
}

func (w *writeCountingCacheClient) IntentDelete(ctx context.Context, intentName string, ignoreNonExisting bool) error {
	w.deleteCalls++
	return w.CacheClientBound.IntentDelete(ctx, intentName, ignoreNonExisting)
}

func newColdRestartedDatastoreFromFixture(t *testing.T, ctrl *gomock.Controller, orig *mowDatastoreFixture) (*Datastore, *mocktarget.MockTarget, *writeCountingCacheClient) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	confirmed := newConfirmedAppliedValues()
	tc := tree.NewTreeContext(orig.ds.schemaClient, vpf, tree.WithConfirmedAppliedValues(confirmed))
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot(restarted): %v", err)
	}
	if err := syncTreeRoot.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("FinishInsertionPhase(restarted): %v", err)
	}

	wrappedCache := &writeCountingCacheClient{
		CacheClientBound: cache.NewCacheClientBound(orig.ds.Name(), orig.localCache),
	}
	sbi := mocktarget.NewMockTarget(ctrl)
	restarted := &Datastore{
		config:           orig.ds.config,
		cacheClient:      wrappedCache,
		sbi:              sbi,
		schemaClient:     orig.ds.schemaClient,
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
	restarted.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(restarted), types.WithAutoRollbackLock(restarted.dmutex))
	return restarted, sbi, wrappedCache
}

func TestDatastore_MutateOnWrite_StandaloneColdStartFirstSyncSeedsFromCache(t *testing.T) {
	ctx := context.Background()

	t.Run("first sync on restarted Datastore seeds pre-existing Non-revertive intents without writing to cacheClient", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		const (
			cleartextPw      = "ColdStartPw1!"
			hashedPw         = "$6$cold1$hashedpw"
			cleartextOverlay = "ColdStartOv1!"
			hashedOverlay    = "$6$cold2$hashedov"
			cleartextAlice   = "ColdStartAlice"
			hashedAlice      = "$6$cold3$hashedalice"
			missingBobPw     = "MissingBobPw1!"
		)

		// Author and confirm a Non-revertive intent on ds1 so localCache holds the persisted intent.
		set1 := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(set1).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{
					{
						Path:  hashedPasswordPath(),
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: hashedPw}},
					},
					{
						Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("system", nil),
							sdcpb.NewPathElem("overlay-password", nil),
						}},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: hashedOverlay}},
					},
					{
						Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("system", nil),
							sdcpb.NewPathElem("user", map[string]string{"username": "bob"}),
							sdcpb.NewPathElem("password", nil),
						}},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$bob$initial"}},
					},
				},
			}},
		}, nil).Times(1)

		nrIntent := buildJSONIntent(t, f.ds, "nr-persisted", 10, true, map[string]any{
			"system": map[string]any{
				"hashed-password":  cleartextPw,
				"overlay-password": cleartextOverlay,
				"hostname":         "router-1",
				"user": []any{
					map[string]any{"username": "alice", "secret": cleartextAlice},
					map[string]any{"username": "bob", "password": missingBobPw},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-init", []*types.TransactionIntent{nrIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-init): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-init"); err != nil {
			t.Fatalf("TransactionConfirm(tx-init): %v", err)
		}

		beforeBlob, err := f.localCache.InstanceIntentGet(ctx, f.ds.Name(), "nr-persisted")
		if err != nil {
			t.Fatalf("InstanceIntentGet(before restart): %v", err)
		}
		beforeRaw := beforeBlob.String()

		// Simulate standalone Datastore cold restart pointing at the same localCache.
		ds2, sbi2, cacheSpy := newColdRestartedDatastoreFromFixture(t, ctrl, f)
		sbi2.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		sbi2.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		// Very first sync on ds2: hashed-password, overlay-password, and alice/secret are present; bob/password is missing.
		if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hashed-password":  hashedPw,
					"overlay-password": hashedOverlay,
					"hostname":         "router-1",
					"user": []any{
						map[string]any{"username": "alice", "secret": hashedAlice},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ds2.ApplyToRunning(cold first sync): %v", err)
		}

		if cacheSpy.modifyCalls != 0 || cacheSpy.deleteCalls != 0 {
			t.Fatalf("cold-start sync wrote to cacheClient: modifyCalls=%d deleteCalls=%d, want 0/0",
				cacheSpy.modifyCalls, cacheSpy.deleteCalls)
		}
		afterBlob, err := f.localCache.InstanceIntentGet(ctx, f.ds.Name(), "nr-persisted")
		if err != nil {
			t.Fatalf("InstanceIntentGet(after cold sync): %v", err)
		}
		if afterBlob.String() != beforeRaw {
			t.Fatalf("persisted intent changed across cold-start sync:\nbefore: %s\nafter:  %s", beforeRaw, afterBlob.String())
		}

		notApplied := map[string]bool{}
		for _, ev := range collectWatchDeviationsBatch(t, ds2) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				notApplied[ev.GetPath().ToXPath(false)] = true
			}
		}
		for _, cleanPath := range []string{
			"/system/hashed-password",
			"/system/overlay-password",
			"/system/user[username=alice]/secret",
		} {
			if notApplied[cleanPath] {
				t.Fatalf("unexpected NOT_APPLIED after cold-start seed on %s (all: %v)", cleanPath, notApplied)
			}
		}
		if !notApplied["/system/user[username=bob]/password"] {
			t.Fatalf("expected NOT_APPLIED on missing /system/user[username=bob]/password, got %v", notApplied)
		}

		// One-shot check: a second sync where bob/password suddenly appears via CLI must NOT seed bob/password.
		if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"hashed-password":  hashedPw,
					"overlay-password": hashedOverlay,
					"hostname":         "router-1",
					"user": []any{
						map[string]any{"username": "alice", "secret": hashedAlice},
						map[string]any{"username": "bob", "password": "$6$cli$bobunexpected"},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ds2.ApplyToRunning(second sync): %v", err)
		}

		foundBobNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, ds2) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/user[username=bob]/password" {
				foundBobNotApplied = true
			}
		}
		if !foundBobNotApplied {
			t.Fatal("expected second sync NOT to cold-seed missing bob/password after CLI edit")
		}
	})

	t.Run("cold-start seeding does not suppress revertive intents", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		expectHashedPasswordSetAndReadback(t, f, "RevertivePw1!", "$6$rev$hash1", nil)
		revIntent := buildJSONIntent(t, f.ds, "rev-persisted", 10, false, map[string]any{
			"system": map[string]any{"hashed-password": "RevertivePw1!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-rev", []*types.TransactionIntent{revIntent}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-rev): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-rev"); err != nil {
			t.Fatalf("TransactionConfirm(tx-rev): %v", err)
		}

		ds2, sbi2, _ := newColdRestartedDatastoreFromFixture(t, ctrl, f)

		// Pure cold start with a revertive intent in cacheClient must still run performRevert + targeted Get.
		revSet := sbi2.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			DoAndReturn(func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
				upds, err := source.ToProtoUpdates(setCtx, true)
				if err != nil {
					return nil, err
				}
				if got := updatesByXPath(upds)["/system/hashed-password"]; got != "RevertivePw1!" {
					t.Fatalf("cold-start revert pushed hashed-password = %q, want RevertivePw1!", got)
				}
				return &sdcpb.SetDataResponse{}, nil
			}).Times(1)
		sbi2.EXPECT().Get(gomock.Any(), gomock.Any()).After(revSet).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{{
					Path:  hashedPasswordPath(),
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$rev$reapplied"}},
				}},
			}},
		}, nil).Times(1)

		if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hashed-password": "$6$rev$hash1"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ds2.ApplyToRunning: %v", err)
		}
	})

	t.Run("cold-start first sync does not overwrite baseline already bound by live pre-sync TransactionSet", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		liveSet := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(liveSet).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{{
					Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
						sdcpb.NewPathElem("system", nil),
						sdcpb.NewPathElem("overlay-password", nil),
					}},
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$live$ovhash1"}},
				}},
			}},
		}, nil).Times(1)

		liveNR := buildJSONIntent(t, f.ds, "nr-live", 10, true, map[string]any{
			"system": map[string]any{"overlay-password": "LiveOverlayPw!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-live", []*types.TransactionIntent{liveNR}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-live): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-live"); err != nil {
			t.Fatalf("TransactionConfirm(tx-live): %v", err)
		}

		// First sync on f.ds delivers a CLI-drifted hash after tx-live finished; preserveExisting keeps "$6$live$ovhash1".
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"overlay-password": "$6$cli$ovdrifted"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning: %v", err)
		}

		foundLiveDrift := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/overlay-password" {
				foundLiveDrift = true
			}
		}
		if !foundLiveDrift {
			t.Fatal("expected cold-start first sync not to overwrite pre-existing in-memory baseline on /system/overlay-password")
		}
	})
}

func TestDatastore_MutateOnWrite_PostSeedOutOfBandDriftRaisesNotApplied(t *testing.T) {
	ctx := context.Background()

	const (
		cleartextPw    = "SeededPw1!"
		seededPwHash   = "$6$seed$pwhash"
		driftedPwHash  = "$6$cli$driftedpw"
		cleartextAlice = "SeededAlice1"
		seededAlice    = "$6$seed$alicehash"
		driftedAlice   = "$6$cli$driftedalice"
	)

	modes := []struct {
		name  string
		setup func(t *testing.T, ctrl *gomock.Controller) *Datastore
	}{
		{
			name: "PreviouslyApplied after initial sync",
			setup: func(t *testing.T, ctrl *gomock.Controller) *Datastore {
				f := newMOWDatastoreFixture(t, ctrl)
				f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)
				if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
					jsonImporter.NewJsonTreeImporter(map[string]any{
						"system": map[string]any{
							"hashed-password": seededPwHash,
							"user": []any{
								map[string]any{"username": "alice", "secret": seededAlice},
							},
						},
					}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
					t.Fatalf("ApplyToRunning(initial): %v", err)
				}
				expectEmptySet(t, f.sbi)
				pa := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-intent", 10, true, map[string]any{
					"system": map[string]any{
						"hashed-password": cleartextPw,
						"user": []any{
							map[string]any{"username": "alice", "secret": cleartextAlice},
						},
					},
				})
				if _, err := f.ds.TransactionSet(ctx, "tx-pa", []*types.TransactionIntent{pa}, nil, time.Minute, false); err != nil {
					t.Fatalf("TransactionSet(tx-pa): %v", err)
				}
				if err := f.ds.TransactionConfirm(ctx, "tx-pa"); err != nil {
					t.Fatalf("TransactionConfirm(tx-pa): %v", err)
				}
				return f.ds
			},
		},
		{
			name: "PreviouslyApplied before initial sync",
			setup: func(t *testing.T, ctrl *gomock.Controller) *Datastore {
				f := newMOWDatastoreFixture(t, ctrl)
				f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)
				expectEmptySet(t, f.sbi)
				pa := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-intent", 10, true, map[string]any{
					"system": map[string]any{
						"hashed-password": cleartextPw,
						"user": []any{
							map[string]any{"username": "alice", "secret": cleartextAlice},
						},
					},
				})
				if _, err := f.ds.TransactionSet(ctx, "tx-pa-pre", []*types.TransactionIntent{pa}, nil, time.Minute, false); err != nil {
					t.Fatalf("TransactionSet(tx-pa-pre): %v", err)
				}
				if err := f.ds.TransactionConfirm(ctx, "tx-pa-pre"); err != nil {
					t.Fatalf("TransactionConfirm(tx-pa-pre): %v", err)
				}
				if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
					jsonImporter.NewJsonTreeImporter(map[string]any{
						"system": map[string]any{
							"hashed-password": seededPwHash,
							"user": []any{
								map[string]any{"username": "alice", "secret": seededAlice},
							},
						},
					}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
					t.Fatalf("ApplyToRunning(initial): %v", err)
				}
				return f.ds
			},
		},
		{
			name: "standalone cold-start first sync from cacheClient",
			setup: func(t *testing.T, ctrl *gomock.Controller) *Datastore {
				f := newMOWDatastoreFixture(t, ctrl)
				set1 := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
				f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(set1).Return(&sdcpb.GetDataResponse{
					Notification: []*sdcpb.Notification{{
						Update: []*sdcpb.Update{{
							Path:  hashedPasswordPath(),
							Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: seededPwHash}},
						}},
					}},
				}, nil).Times(1)
				nr := buildJSONIntent(t, f.ds, "nr-intent", 10, true, map[string]any{
					"system": map[string]any{
						"hashed-password": cleartextPw,
						"user": []any{
							map[string]any{"username": "alice", "secret": cleartextAlice},
						},
					},
				})
				if _, err := f.ds.TransactionSet(ctx, "tx-init", []*types.TransactionIntent{nr}, nil, time.Minute, false); err != nil {
					t.Fatalf("TransactionSet(tx-init): %v", err)
				}
				if err := f.ds.TransactionConfirm(ctx, "tx-init"); err != nil {
					t.Fatalf("TransactionConfirm(tx-init): %v", err)
				}
				ds2, sbi2, _ := newColdRestartedDatastoreFromFixture(t, ctrl, f)
				sbi2.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
				sbi2.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)
				if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
					jsonImporter.NewJsonTreeImporter(map[string]any{
						"system": map[string]any{
							"hashed-password": seededPwHash,
							"user": []any{
								map[string]any{"username": "alice", "secret": seededAlice},
							},
						},
					}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
					t.Fatalf("ds2.ApplyToRunning(first sync): %v", err)
				}
				return ds2
			},
		},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ds := mode.setup(t, ctrl)

			// Baseline is converged immediately after seeding.
			for _, ev := range collectWatchDeviationsBatch(t, ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
					t.Fatalf("unexpected NOT_APPLIED right after seeding: %v", ev)
				}
			}

			// Out-of-band CLI edit changing Running away from seeded hashes must raise NOT_APPLIED.
			if err := ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{
						"hashed-password": driftedPwHash,
						"user": []any{
							map[string]any{"username": "alice", "secret": driftedAlice},
						},
					},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(CLI value drift): %v", err)
			}

			editedDevs := map[string]*sdcpb.WatchDeviationResponse{}
			for _, ev := range collectWatchDeviationsBatch(t, ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
					editedDevs[ev.GetPath().ToXPath(false)] = ev
				}
			}
			pwDev := editedDevs["/system/hashed-password"]
			if pwDev == nil || pwDev.GetExpectedValue().GetStringVal() != cleartextPw || pwDev.GetCurrentValue().GetStringVal() != driftedPwHash {
				t.Fatalf("CLI edit deviation on /system/hashed-password = %v, want expected=%q current=%q", pwDev, cleartextPw, driftedPwHash)
			}
			aliceDev := editedDevs["/system/user[username=alice]/secret"]
			if aliceDev == nil || aliceDev.GetExpectedValue().GetStringVal() != cleartextAlice || aliceDev.GetCurrentValue().GetStringVal() != driftedAlice {
				t.Fatalf("CLI edit deviation on /system/user[username=alice]/secret = %v, want expected=%q current=%q", aliceDev, cleartextAlice, driftedAlice)
			}

			// Out-of-band CLI deletion removing both leaves from Running must also raise NOT_APPLIED.
			if err := ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
				jsonImporter.NewJsonTreeImporter(map[string]any{
					"system": map[string]any{"hostname": "after-delete"},
				}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
				t.Fatalf("ApplyToRunning(CLI delete): %v", err)
			}

			deletedDevs := map[string]*sdcpb.WatchDeviationResponse{}
			for _, ev := range collectWatchDeviationsBatch(t, ds) {
				if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
					deletedDevs[ev.GetPath().ToXPath(false)] = ev
				}
			}
			if d := deletedDevs["/system/hashed-password"]; d == nil || d.GetCurrentValue() != nil {
				t.Fatalf("CLI delete deviation on /system/hashed-password = %v, want NOT_APPLIED with nil CurrentValue", d)
			}
			if d := deletedDevs["/system/user[username=alice]/secret"]; d == nil || d.GetCurrentValue() != nil {
				t.Fatalf("CLI delete deviation on /system/user[username=alice]/secret = %v, want NOT_APPLIED with nil CurrentValue", d)
			}
		})
	}

	t.Run("cancelling unconfirmed PreviouslyApplied transaction before initial sync evicts awaitingInitialSyncSeed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		expectEmptySet(t, f.sbi)
		pa := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-cancelled", 15, true, map[string]any{
			"system": map[string]any{
				"user": []any{
					map[string]any{"username": "alice", "secret": "CancelledAlice1"},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-unconfirmed", []*types.TransactionIntent{pa}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-unconfirmed): %v", err)
		}

		// Rollback while Running == nil sends an empty southbound diff and must evict awaitingInitialSyncSeed.
		expectEmptySet(t, f.sbi)
		if err := f.ds.TransactionCancel(ctx, "tx-unconfirmed"); err != nil {
			t.Fatalf("TransactionCancel(tx-unconfirmed): %v", err)
		}

		// Initial sync arrives with unmanaged alice secret on target; because tx-unconfirmed was cancelled,
		// /system/user/secret must NOT be learned from the cancelled awaitingInitialSyncSeed entry.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{
						map[string]any{"username": "alice", "secret": "$6$unmanaged$alicehash"},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(initial): %v", err)
		}

		// Subsequent TransactionSet on bob/secret must issue Set only and zero targeted Gets.
		f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		bob := buildJSONIntent(t, f.ds, "bob-intent", 10, false, map[string]any{
			"system": map[string]any{
				"user": []any{
					map[string]any{"username": "bob", "secret": "BobPlain1"},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-bob", []*types.TransactionIntent{bob}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-bob): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-bob"); err != nil {
			t.Fatalf("TransactionConfirm(tx-bob): %v", err)
		}
	})
}

func TestDatastore_MutateOnWrite_PreviouslyAppliedLongCryptAndChoiceLeaf(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	const (
		longAliceHash     = "$6$rounds=5000$alicesalt$0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		longUnmanagedHash = "$6$rounds=5000$othersalt$fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
		primaryHash       = "$6$choice$primaryhash"
		fallbackHash      = "$6$choice$fallbackhash"
	)

	aliceSecretPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
		sdcpb.NewPathElem("system", nil),
		sdcpb.NewPathElem("user", map[string]string{"username": "alice"}),
		sdcpb.NewPathElem("secret", nil),
	}}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"primary-auth": primaryHash},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(primary): %v", err)
	}
	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{
				"fallback-auth": fallbackHash,
				"user": []any{
					map[string]any{"username": "alice", "secret": longAliceHash},
					map[string]any{"username": "bob", "secret": "DriftedPlainBob"},
					map[string]any{"username": "unmanaged", "secret": longUnmanagedHash},
				},
			},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(fallback + long crypt): %v", err)
	}

	recoveredDryRun := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-choice-crypt", 10, true, map[string]any{
		"system": map[string]any{
			"fallback-auth": "ChoiceCleartext1!",
			"user": []any{
				map[string]any{"username": "alice", "secret": "AlicePlain123"},
				map[string]any{"username": "bob", "secret": "BobPlain123"},
			},
		},
	})
	dryResp, err := f.ds.TransactionSet(ctx, "tx-dry", []*types.TransactionIntent{recoveredDryRun}, nil, time.Minute, true)
	if err != nil {
		t.Fatalf("TransactionSet(dryRun) err = %v", err)
	}
	if len(dryResp.GetErrors()) != 0 {
		t.Fatalf("TransactionSet(dryRun) validation errors = %v, want none", dryResp.GetErrors())
	}
	if f.ds.confirmedApplied.IsLearnedMutateOnWritePath(aliceSecretPath) {
		t.Fatal("dryRun must not mutate learned mutate-on-write paths")
	}

	expectEmptySet(t, f.sbi)
	recovered := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-choice-crypt", 10, true, map[string]any{
		"system": map[string]any{
			"fallback-auth": "ChoiceCleartext1!",
			"user": []any{
				map[string]any{"username": "alice", "secret": "AlicePlain123"},
				map[string]any{"username": "bob", "secret": "BobPlain123"},
			},
		},
	})
	resp, err := f.ds.TransactionSet(ctx, "tx-commit", []*types.TransactionIntent{recovered}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(commit) err = %v", err)
	}
	if len(resp.GetErrors()) != 0 {
		t.Fatalf("TransactionSet(commit) validation errors = %v, want none", resp.GetErrors())
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-commit"); err != nil {
		t.Fatalf("TransactionConfirm(tx-commit): %v", err)
	}

	if !f.ds.confirmedApplied.IsLearnedMutateOnWritePath(aliceSecretPath) {
		t.Fatal("commit of PreviouslyApplied crypt leaf must permanently learn keyless schema path")
	}

	notApplied := map[string]bool{}
	for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
			notApplied[ev.GetPath().ToXPath(false)] = true
		}
	}
	if notApplied["/system/fallback-auth"] || notApplied["/system/user[username=alice]/secret"] {
		t.Fatalf("unexpected NOT_APPLIED on choice or crypt leaf: %v", notApplied)
	}
	if !notApplied["/system/user[username=bob]/secret"] {
		t.Fatalf("expected NOT_APPLIED on sibling bob with drifted non-crypt plain string, got %v", notApplied)
	}

	// Subsequent live Set (without PreviouslyApplied) must pass validation against >32-char Running hashes
	// and perform a post-Set readback on the learned user/secret path.
	carolSecretPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
		sdcpb.NewPathElem("system", nil),
		sdcpb.NewPathElem("user", map[string]string{"username": "carol"}),
		sdcpb.NewPathElem("secret", nil),
	}}
	setCarol := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(setCarol).Return(&sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{{
			Update: []*sdcpb.Update{{
				Path:  carolSecretPath,
				Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$carol$hash"}},
			}},
		}},
	}, nil).Times(1)
	liveCarol := buildJSONIntent(t, f.ds, "live-carol", 5, true, map[string]any{
		"system": map[string]any{
			"user": []any{
				map[string]any{"username": "carol", "secret": "CarolPlain123"},
			},
		},
	})
	liveResp, err := f.ds.TransactionSet(ctx, "tx-live-carol", []*types.TransactionIntent{liveCarol}, nil, time.Minute, false)
	if err != nil {
		t.Fatalf("TransactionSet(tx-live-carol): %v", err)
	}
	if len(liveResp.GetErrors()) != 0 {
		t.Fatalf("TransactionSet(tx-live-carol) validation errors = %v, want none", liveResp.GetErrors())
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-live-carol"); err != nil {
		t.Fatalf("TransactionConfirm(tx-live-carol): %v", err)
	}
}

func TestDatastore_MutateOnWrite_MultiSubtreeColdStartAndPostStartupIsolation(t *testing.T) {
	ctx := context.Background()

	t.Run("multi-subtree cold start seeds each pre-existing Non-revertive subtree when its sync arrives", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		set1 := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(set1).Return(&sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{{
				Update: []*sdcpb.Update{
					{
						Path:  hashedPasswordPath(),
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$sys$hash"}},
					},
					{
						Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
							sdcpb.NewPathElem("extra", nil),
							sdcpb.NewPathElem("token", nil),
						}},
						Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$extra$hash"}},
					},
				},
			}},
		}, nil).Times(1)

		nr := buildJSONIntent(t, f.ds, "nr-multisub", 10, true, map[string]any{
			"system": map[string]any{"hashed-password": "SysSecret1!"},
			"extra":  map[string]any{"token": "ExtraToken1!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-init", []*types.TransactionIntent{nr}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-init): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-init"); err != nil {
			t.Fatalf("TransactionConfirm(tx-init): %v", err)
		}

		ds2, sbi2, cacheSpy := newColdRestartedDatastoreFromFixture(t, ctrl, f)
		sbi2.EXPECT().Set(gomock.Any(), gomock.Any()).Times(0)
		sbi2.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)

		// Subtree 1 (/extra) syncs first.
		extraPath := []*sdcpb.Path{{IsRootBased: true, Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("extra", nil)}}}
		if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), extraPath,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"extra": map[string]any{"token": "$6$extra$hash"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(/extra): %v", err)
		}

		// Subtree 2 (/system) syncs second: must ALSO seed /system/hashed-password cleanly.
		sysPath := []*sdcpb.Path{{IsRootBased: true, Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("system", nil)}}}
		if err := ds2.ApplyToRunning(ctx, time.Now().UnixNano(), sysPath,
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hashed-password": "$6$sys$hash"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(/system): %v", err)
		}

		if cacheSpy.modifyCalls != 0 || cacheSpy.deleteCalls != 0 {
			t.Fatalf("unexpected cache writes during multi-subtree cold start: modify=%d delete=%d",
				cacheSpy.modifyCalls, cacheSpy.deleteCalls)
		}

		for _, ev := range collectWatchDeviationsBatch(t, ds2) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
				t.Fatalf("unexpected NOT_APPLIED after multi-subtree cold start: %v", ev)
			}
		}
	})

	t.Run("fresh empty-cache Datastore never cold-seeds post-startup promoted Non-revertive owners on first sync", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		expectHashedPasswordSetAndReadback(t, f, "HighPw1!", "$6$high$hash", nil)
		high := buildJSONIntent(t, f.ds, "high-owner", 5, false, map[string]any{
			"system": map[string]any{"hashed-password": "HighPw1!"},
		})
		midNR := buildJSONIntent(t, f.ds, "mid-nr", 15, true, map[string]any{
			"system": map[string]any{"hashed-password": "MidOtherPw!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-1", []*types.TransactionIntent{high, midNR}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-1): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-1"); err != nil {
			t.Fatalf("TransactionConfirm(tx-1): %v", err)
		}

		// Demote high-owner before first sync runs so mid-nr becomes active without baseline.
		expectEmptySet(t, f.sbi)
		demoted := buildJSONIntent(t, f.ds, "high-owner", 25, false, map[string]any{
			"system": map[string]any{"hashed-password": "HighPw1!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-2", []*types.TransactionIntent{demoted}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-2): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-2"); err != nil {
			t.Fatalf("TransactionConfirm(tx-2): %v", err)
		}

		// First sync arrives on this fresh Datastore: mid-nr was NOT a cold-start intent and must report NOT_APPLIED.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{"hashed-password": "$6$high$hash"},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(first sync): %v", err)
		}

		foundNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetIntent() == "mid-nr" &&
				ev.GetPath().ToXPath(false) == "/system/hashed-password" {
				foundNotApplied = true
			}
		}
		if !foundNotApplied {
			t.Fatal("expected post-startup promoted mid-nr not to be cold-seeded on first sync")
		}
	})
}

func TestDatastore_MutateOnWrite_InitialSyncCompletionClearsAbsentSeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)
	expectEmptySet(t, f.sbi)

	recovered := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-stream", 10, true, map[string]any{
		"system": map[string]any{
			"hashed-password":  "StreamPresent1!",
			"overlay-password": "StreamAbsent1!",
		},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-stream-pa", []*types.TransactionIntent{recovered}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet: %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-stream-pa"); err != nil {
		t.Fatalf("TransactionConfirm: %v", err)
	}

	if err := f.ds.ApplyInitialSync(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, nil,
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": "$6$stream$present"},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyInitialSync: %v", err)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), nil,
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"overlay-password": "$6$stream$latercli"},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(later stream CLI): %v", err)
	}

	notApplied := map[string]bool{}
	for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
			notApplied[ev.GetPath().ToXPath(false)] = true
		}
	}
	if notApplied["/system/hashed-password"] {
		t.Fatalf("unexpected NOT_APPLIED on /system/hashed-password: %v", notApplied)
	}
	if !notApplied["/system/overlay-password"] {
		t.Fatalf("expected NOT_APPLIED on /system/overlay-password after initial stream sync confirmed it absent, got %v", notApplied)
	}
}

func TestDatastore_MutateOnWrite_CompleteInitialSyncPreservesPostSetPendingCapture(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	setCall := f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).After(setCall).Return(nil, errors.New("readback timeout")).Times(1)
	live := buildJSONIntent(t, f.ds, "nr-live", 10, true, map[string]any{
		"system": map[string]any{"hashed-password": "LivePlainPw1!"},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-live", []*types.TransactionIntent{live}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-live): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-live"); err != nil {
		t.Fatalf("TransactionConfirm(tx-live): %v", err)
	}

	if err := f.ds.ApplyInitialSync(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, nil, nil); err != nil {
		t.Fatalf("ApplyInitialSync: %v", err)
	}
	if got := f.ds.confirmedApplied.Lookup(
		hashedPasswordPath(),
		"nr-live",
		&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "LivePlainPw1!"}},
	); got != nil {
		t.Fatalf("ApplyInitialSync falsely locked optimistic cleartext into baseline: %v", got)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
		jsonImporter.NewJsonTreeImporter(map[string]any{
			"system": map[string]any{"hashed-password": "$6$live$realhash"},
		}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
		t.Fatalf("ApplyToRunning(real sync): %v", err)
	}

	for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
		if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE && ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED {
			t.Fatalf("unexpected NOT_APPLIED after real sync captured hash: %v", ev)
		}
	}
}

func TestDatastore_MutateOnWrite_ExplicitDeleteOnUpdatedHighOwnerOverrulesPreviouslyApplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	f := newMOWDatastoreFixture(t, ctrl)
	ctx := context.Background()

	f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, errors.New("offline")).Times(1)

	highInit := buildJSONIntent(t, f.ds, "high-owner", 5, false, map[string]any{
		"system": map[string]any{"hashed-password": "HighPw1!"},
	})
	if _, err := f.ds.TransactionSet(ctx, "tx-high-init", []*types.TransactionIntent{highInit}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-high-init): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-high-init"); err != nil {
		t.Fatalf("TransactionConfirm(tx-high-init): %v", err)
	}

	if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}}, nil); err != nil {
		t.Fatalf("ApplyToRunning(clear): %v", err)
	}
	f.ds.syncTreeMutex.Lock()
	f.ds.syncedPaths = nil
	f.ds.syncTreeMutex.Unlock()

	f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).Return(&sdcpb.SetDataResponse{}, nil).Times(1)
	f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{{
			Update: []*sdcpb.Update{{
				Path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("extra", nil),
					sdcpb.NewPathElem("token", nil),
				}},
				Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "$6$tok$hash"}},
			}},
		}},
	}, nil).Times(1)

	highUpdated := buildJSONIntent(t, f.ds, "high-owner", 5, false, map[string]any{
		"extra": map[string]any{"token": "HighTok1!"},
	})
	highUpdated.AddExplicitDeletes([]*sdcpb.Path{hashedPasswordPath()})

	lowPA := buildPreviouslyAppliedJSONIntent(t, f.ds, "low-pa", 50, true, map[string]any{
		"system": map[string]any{"hashed-password": "LowOverruledPw1!"},
	})

	if _, err := f.ds.TransactionSet(ctx, "tx-explicit-del", []*types.TransactionIntent{highUpdated, lowPA}, nil, time.Minute, false); err != nil {
		t.Fatalf("TransactionSet(tx-explicit-del): %v", err)
	}
	if err := f.ds.TransactionConfirm(ctx, "tx-explicit-del"); err != nil {
		t.Fatalf("TransactionConfirm(tx-explicit-del): %v", err)
	}

	for _, epoch := range f.ds.confirmedApplied.CausalCaptures() {
		if newCanonicalLeafPath(epoch.path) == newCanonicalLeafPath(hashedPasswordPath()) && epoch.owner == "low-pa" {
			t.Fatalf("overruled low-pa must not arm initial-sync seed under explicit delete by high-owner: %+v", epoch)
		}
	}
}

func TestDatastore_MutateOnWrite_KeylessListSyncPathAndRollbackPreservation(t *testing.T) {
	ctx := context.Background()

	t.Run("keyless intermediate list sync path clears absent awaitingInitialSyncSeed and records syncedPaths", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		f.sbi.EXPECT().Get(gomock.Any(), gomock.Any()).Times(0)
		expectEmptySet(t, f.sbi)

		pa := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-list-pa", 10, true, map[string]any{
			"system": map[string]any{
				"user": []any{
					map[string]any{"username": "alice", "secret": "AlicePlain123"},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-pa-1", []*types.TransactionIntent{pa}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-pa-1): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-pa-1"); err != nil {
			t.Fatalf("TransactionConfirm(tx-pa-1): %v", err)
		}

		// Initial sync configured on keyless list leaf path /system/user/secret finds target empty.
		keylessSecretSyncPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("system", nil),
			sdcpb.NewPathElem("user", nil),
			sdcpb.NewPathElem("secret", nil),
		}}
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{keylessSecretSyncPath}, nil); err != nil {
			t.Fatalf("ApplyToRunning(empty keyless list sync): %v", err)
		}

		// Replaying PreviouslyApplied after /system/user/secret synced must also see syncedPaths coverage
		// and NOT re-arm awaitingInitialSyncSeed.
		expectEmptySet(t, f.sbi)
		paReplay := buildPreviouslyAppliedJSONIntent(t, f.ds, "nr-list-pa", 10, true, map[string]any{
			"system": map[string]any{
				"user": []any{
					map[string]any{"username": "alice", "secret": "AlicePlain123"},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-pa-2", []*types.TransactionIntent{paReplay}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-pa-2): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-pa-2"); err != nil {
			t.Fatalf("TransactionConfirm(tx-pa-2): %v", err)
		}

		// Subsequent out-of-band CLI edit adding alice's secret on target must raise NOT_APPLIED.
		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{keylessSecretSyncPath},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"user": []any{
						map[string]any{"username": "alice", "secret": "$6$cli$alicehash"},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning(later CLI edit): %v", err)
		}

		foundNotApplied := false
		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/user[username=alice]/secret" {
				foundNotApplied = true
			}
		}
		if !foundNotApplied {
			t.Fatal("expected NOT_APPLIED on /system/user[username=alice]/secret after keyless list sync confirmed it absent")
		}
	})

	t.Run("canceling unconfirmed PreviouslyApplied transaction restores prior Non-revertive baseline and learnedPaths", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		f := newMOWDatastoreFixture(t, ctrl)

		if err := f.ds.ApplyToRunning(ctx, time.Now().UnixNano(), []*sdcpb.Path{{}},
			jsonImporter.NewJsonTreeImporter(map[string]any{
				"system": map[string]any{
					"fallback-auth": "$6$fallback$seeded",
					"user": []any{
						map[string]any{"username": "alice", "secret": "$6$alice$unconfirmed"},
					},
				},
			}, consts.RunningIntentName, consts.RunningValuesPrio, false)); err != nil {
			t.Fatalf("ApplyToRunning: %v", err)
		}

		expectEmptySet(t, f.sbi)
		lowNR := buildPreviouslyAppliedJSONIntent(t, f.ds, "low-nr", 20, true, map[string]any{
			"system": map[string]any{"fallback-auth": "LowFallback1!"},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-low", []*types.TransactionIntent{lowNR}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-low): %v", err)
		}
		if err := f.ds.TransactionConfirm(ctx, "tx-low"); err != nil {
			t.Fatalf("TransactionConfirm(tx-low): %v", err)
		}

		assertFallbackUntouched := func(setCtx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
			upds, err := source.ToProtoUpdates(setCtx, true)
			if err != nil {
				return nil, err
			}
			for _, u := range upds {
				if u.GetPath().ToXPath(false) == "/system/fallback-auth" {
					t.Fatalf("unexpected southbound update on /system/fallback-auth: %v", u)
				}
			}
			return &sdcpb.SetDataResponse{}, nil
		}
		f.sbi.EXPECT().Set(gomock.Any(), gomock.Any()).DoAndReturn(assertFallbackUntouched).Times(2)

		highPA := buildPreviouslyAppliedJSONIntent(t, f.ds, "high-pa", 5, true, map[string]any{
			"system": map[string]any{
				"primary-auth": "HighPrimaryPw!",
				"user": []any{
					map[string]any{"username": "alice", "secret": "AliceCancelPw!"},
				},
			},
		})
		if _, err := f.ds.TransactionSet(ctx, "tx-cancel-pa", []*types.TransactionIntent{highPA}, nil, time.Minute, false); err != nil {
			t.Fatalf("TransactionSet(tx-cancel-pa): %v", err)
		}
		if err := f.ds.TransactionCancel(ctx, "tx-cancel-pa"); err != nil {
			t.Fatalf("TransactionCancel(tx-cancel-pa): %v", err)
		}

		aliceSecretPath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("system", nil),
			sdcpb.NewPathElem("user", map[string]string{"username": "alice"}),
			sdcpb.NewPathElem("secret", nil),
		}}
		if f.ds.confirmedApplied.IsLearnedMutateOnWritePath(aliceSecretPath) {
			t.Fatal("canceled PreviouslyApplied transaction must not leak learned mutate-on-write paths")
		}

		for _, ev := range collectWatchDeviationsBatch(t, f.ds) {
			if ev.GetEvent() == sdcpb.DeviationEvent_UPDATE &&
				ev.GetReason() == sdcpb.DeviationReason_NOT_APPLIED &&
				ev.GetPath().ToXPath(false) == "/system/fallback-auth" {
				t.Fatalf("canceled PreviouslyApplied transaction lost low-nr fallback-auth baseline: %v", ev)
			}
		}
	})
}







