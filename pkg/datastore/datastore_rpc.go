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
	"errors"
	"sync"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	status "google.golang.org/grpc/status"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
)

type Datastore struct {
	// datastore config
	config *config.DatastoreConfig

	cacheClient cache.CacheClientBound

	// SBI target of this datastore
	sbi target.Target

	// schema server client
	// schemaClient sdcpb.SchemaServerClient
	schemaClient schemaClient.SchemaClientBound

	// sync channel, to be passed to the SBI Sync method
	synCh chan *target.SyncUpdate

	// stop cancel func
	cfn context.CancelFunc

	// keeps track of clients watching deviation updates
	m                *sync.RWMutex
	deviationClients map[string]sdcpb.DataServer_WatchDeviationsServer

	// per path intent deviations (no unhandled)
	md                       *sync.RWMutex
	currentIntentsDeviations map[string][]*sdcpb.WatchDeviationResponse

	// datastore mutex locks the whole datasore for further set operations
	dmutex *sync.Mutex

	// TransactionManager
	transactionManager *types.TransactionManager

	// SyncTree
	syncTree      *tree.RootEntry
	syncTreeMutex *sync.RWMutex

	// owned by sync
	syncTreeCandidat *tree.RootEntry
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(ctx context.Context, c *config.DatastoreConfig, sc schema.Client, cc cache.Client, opts ...grpc.DialOption) (*Datastore, error) {

	scb := schemaClient.NewSchemaClientBound(c.Schema, sc)
	tc := tree.NewTreeContext(scb, tree.RunningIntentName)
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}

	ccb := cache.NewCacheClientBound(c.Name, cc)

	ds := &Datastore{
		config:                   c,
		schemaClient:             scb,
		cacheClient:              ccb,
		m:                        &sync.RWMutex{},
		md:                       &sync.RWMutex{},
		dmutex:                   &sync.Mutex{},
		deviationClients:         make(map[string]sdcpb.DataServer_WatchDeviationsServer),
		currentIntentsDeviations: make(map[string][]*sdcpb.WatchDeviationResponse),
		syncTree:                 syncTreeRoot,
		syncTreeMutex:            &sync.RWMutex{},
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

	if c.Sync != nil {
		ds.synCh = make(chan *target.SyncUpdate, c.Sync.Buffer)
	}
	ctx, cancel := context.WithCancel(ctx)
	ds.cfn = cancel

	// create cache instance if needed
	// this is a blocking call
	ds.initCache(ctx)

	go func() {
		// init sbi, this is a blocking call
		err := ds.connectSBI(ctx, opts...)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			log.Errorf("failed to create SBI for target %s: %v", ds.Config().Name, err)
			return
		}
		// start syncing goroutine
		if c.Sync != nil {
			go ds.Sync(ctx)
		}
		// start deviation goroutine
		ds.DeviationMgr(ctx)
	}()
	return ds, nil
}

func (d *Datastore) IntentsList(ctx context.Context) ([]string, error) {
	return d.cacheClient.IntentsList(ctx)
}

func (d *Datastore) initCache(ctx context.Context) {

	exists := d.cacheClient.InstanceExists(ctx)
	if exists {
		log.Debugf("cache %q already exists", d.config.Name)
		return
	}
	log.Infof("cache %s does not exist, creating it", d.config.Name)
CREATE:
	err := d.cacheClient.InstanceCreate(ctx)
	if err != nil {
		log.Errorf("failed to create cache %s: %v", d.config.Name, err)
		time.Sleep(time.Second)
		goto CREATE
	}
}

func (d *Datastore) connectSBI(ctx context.Context, opts ...grpc.DialOption) error {
	var err error
	d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, opts...)
	if err == nil {
		return nil
	}

	log.Errorf("failed to create DS %s target: %v", d.config.Name, err)
	ticker := time.NewTicker(d.config.SBI.ConnectRetry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, opts...)
			if err != nil {
				log.Errorf("failed to create DS %s target: %v", d.config.Name, err)
				continue
			}
			return nil
		}
	}
}

func (d *Datastore) Name() string {
	return d.config.Name
}

func (d *Datastore) Schema() *config.SchemaConfig {
	return d.config.Schema
}

func (d *Datastore) Config() *config.DatastoreConfig {
	return d.config
}

func (d *Datastore) Delete(ctx context.Context) error {
	return d.cacheClient.InstanceDelete(ctx)
}

func (d *Datastore) ConnectionState() *target.TargetStatus {
	if d.sbi == nil {
		return target.NewTargetStatus(target.TargetStatusNotConnected)
	}
	return d.sbi.Status()
}

func (d *Datastore) Stop() error {
	if d == nil {
		return nil
	}
	d.cfn()
	if d.sbi == nil {
		return nil
	}
	err := d.sbi.Close()
	if err != nil {
		log.Errorf("datastore %s failed to close the target connection: %v", d.Name(), err)
	}
	return nil
}

func (d *Datastore) Sync(ctx context.Context) {
	go d.sbi.Sync(ctx,
		d.config.Sync,
		d.synCh,
	)

	var err error
	var startTs int64

	d.syncTreeCandidat, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
	if err != nil {
		log.Errorf("creating a new synctree candidate: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				log.Errorf("datastore %s sync stopped: %v", d.Name(), ctx.Err())
			}
			return
		case syncup := <-d.synCh:
			switch {
			case syncup.Start:
				log.Debugf("%s: sync start", d.Name())
				startTs = time.Now().Unix()

			case syncup.End:
				log.Debugf("%s: sync end", d.Name())

				startTs = 0

				d.syncTreeMutex.Lock()
				d.syncTree = d.syncTreeCandidat
				d.syncTreeMutex.Unlock()

				// create new syncTreeCandidat
				d.syncTreeCandidat, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
				if err != nil {
					log.Errorf("creating a new synctree candidate: %v", err)
					return
				}

				// export and write to cache
				runningExport, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio)
				if err != nil {
					log.Error(err)
					continue
				}
				err = d.cacheClient.IntentModify(ctx, runningExport)
				if err != nil {
					log.Errorf("issue modifying running cache content: %v", err)
					continue
				}
			default:
				if startTs == 0 {
					startTs = time.Now().Unix()
				}
				err := d.writeToSyncTreeCandidate(ctx, syncup.Update.GetUpdate(), startTs)
				if err != nil {
					log.Errorf("failed to write to sync tree: %v", err)
				}
			}
		}
	}
}

func (d *Datastore) writeToSyncTreeCandidate(ctx context.Context, updates []*sdcpb.Update, ts int64) error {
	upds, err := treetypes.ExpandAndConvertIntent(ctx, d.schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, updates, ts)
	if err != nil {
		return err
	}

	for _, upd := range upds {
		_, err := d.syncTreeCandidat.AddUpdateRecursive(ctx, upd, treetypes.NewUpdateInsertFlags())
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Datastore) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	d.m.Lock()
	defer d.m.Unlock()

	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	}
	pName := p.Addr.String()

	if oStream, ok := d.deviationClients[pName]; ok {
		_ = oStream // TODO:
	}

	d.deviationClients[pName] = stream
	return nil
}

func (d *Datastore) StopDeviationsWatch(peer string) {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.deviationClients, peer)
}

func (d *Datastore) DeviationMgr(ctx context.Context) {
	log.Infof("%s: starting deviationMgr...", d.Name())
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.m.RLock()
			deviationClients := make([]sdcpb.DataServer_WatchDeviationsServer, 0, len(d.deviationClients))
			for _, devStream := range d.deviationClients {
				deviationClients = append(deviationClients, devStream)
			}
			d.m.RUnlock()
			if len(deviationClients) == 0 {
				continue
			}
			for _, dc := range deviationClients {
				dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_START,
				})
			}
			deviationChan, err := d.calculateDeviations(ctx)
			if err != nil {
				log.Error(err)
				continue
			}
			d.SendDeviations(deviationChan, deviationClients)
			for _, dc := range deviationClients {
				dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_END,
				})
			}
		}
	}
}

func (d *Datastore) SendDeviations(ch <-chan *treetypes.DeviationEntry, deviationClients []sdcpb.DataServer_WatchDeviationsServer) {
	wg := &sync.WaitGroup{}
	for {
		select {
		case de, ok := <-ch:
			if !ok {
				wg.Wait()
				return
			}
			wg.Add(1)
			go func(de DeviationEntry, dcs []sdcpb.DataServer_WatchDeviationsServer) {
				for _, dc := range dcs {
					dc.Send(&sdcpb.WatchDeviationResponse{
						Name:          d.config.Name,
						Intent:        de.IntentName(),
						Event:         sdcpb.DeviationEvent_UPDATE,
						Reason:        sdcpb.DeviationReason(de.Reason()),
						Path:          de.Path(),
						ExpectedValue: de.ExpectedValue(),
						CurrentValue:  de.CurrentValue(),
					})
				}
				wg.Done()
			}(de, deviationClients)
		}
	}
}

type DeviationEntry interface {
	IntentName() string
	Reason() treetypes.DeviationReason
	Path() *sdcpb.Path
	CurrentValue() *sdcpb.TypedValue
	ExpectedValue() *sdcpb.TypedValue
}

func (d *Datastore) calculateDeviations(ctx context.Context) (<-chan *treetypes.DeviationEntry, error) {
	deviationChan := make(chan *treetypes.DeviationEntry, 10)

	d.syncTreeMutex.RLock()
	deviationTree, err := d.syncTree.DeepCopy(ctx)
	if err != nil {
		return nil, err
	}
	d.syncTreeMutex.RUnlock()

	addedIntentNames, err := d.LoadAllButRunningIntents(ctx, deviationTree)
	if err != nil {
		return nil, err
	}

	// Send IntentExists
	for _, n := range addedIntentNames {
		deviationChan <- treetypes.NewDeviationEntry(n, treetypes.DeviationReasonIntentExists, nil)
	}

	err = deviationTree.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		deviationTree.GetDeviations(deviationChan)
		close(deviationChan)
	}()

	return deviationChan, nil
}

// DatastoreRollbackAdapter implements the types.RollbackInterface and encapsulates the Datastore.
type DatastoreRollbackAdapter struct {
	d *Datastore
}

// NewDatastoreRollbackAdapter constructor for the DatastoreRollbackAdapter.
func NewDatastoreRollbackAdapter(d *Datastore) *DatastoreRollbackAdapter {
	return &DatastoreRollbackAdapter{
		d: d,
	}
}

// TransactionRollback is adapted to the datastore.lowlevelTransactionSet() function
func (dra *DatastoreRollbackAdapter) TransactionRollback(ctx context.Context, transaction *types.Transaction, dryRun bool) (*sdcpb.TransactionSetResponse, error) {
	return dra.d.lowlevelTransactionSet(ctx, transaction, dryRun)
}

// Assure the types.RollbackInterface is implemented by the DatastoreRollbackAdapter
var _ types.RollbackInterface = &DatastoreRollbackAdapter{}
