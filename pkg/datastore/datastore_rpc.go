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
	"fmt"
	"strings"
	"sync"
	"time"

	logf "github.com/sdcio/data-server/pkg/log"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
	syncTreeCandidate *tree.RootEntry
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(ctx context.Context, c *config.DatastoreConfig, sc schema.Client, cc cache.Client, opts ...grpc.DialOption) (*Datastore, error) {

	log := logf.FromContext(ctx)
	log = log.WithName("datastore").WithValues(
		"datastore-name", c.Name,
		"target-name", c.Name,
		"schema-name", c.Schema.Name,
		"schema-vendor", c.Schema.Vendor,
		"schema-version", c.Schema.Version,
		"sbi-type", c.SBI.Type,
		"sbi-address", c.SBI.Address,
		"sbi-port", c.SBI.Port,
	)
	ctx = logf.IntoContext(ctx, log)

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
			log.Error(err, "failed to create SBI")
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
	log := logf.FromContext(ctx)

	exists := d.cacheClient.InstanceExists(ctx)
	if exists {
		log.V(logf.VDebug).Info("cache already exists")
		return
	}
	log.Info("creating cache instance")
CREATE:
	err := d.cacheClient.InstanceCreate(ctx)
	if err != nil {
		log.Error(err, "failed to create cache")
		time.Sleep(time.Second)
		goto CREATE
	}
}

func (d *Datastore) connectSBI(ctx context.Context, opts ...grpc.DialOption) error {
	log := logf.FromContext(ctx)

	var err error
	d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, opts...)
	if err == nil {
		return nil
	}

	log.Error(err, "failed to create DS target")
	ticker := time.NewTicker(d.config.SBI.ConnectRetry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, opts...)
			if err != nil {
				log.Error(err, "failed to create DS target")
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
		return fmt.Errorf("datastore %s failed to close the target connection: %v", d.Name(), err)
	}
	return nil
}

func (d *Datastore) Sync(ctx context.Context) {
	log := logf.FromContext(ctx).WithName("sync")
	ctx = logf.IntoContext(ctx, log)

	go d.sbi.Sync(ctx,
		d.config.Sync,
		d.synCh,
	)

	var err error
	var startTs int64

	d.syncTreeCandidate, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
	if err != nil {
		log.Error(err, "failed creating a new synctree candidate")
		return
	}

	for {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				log.Error(ctx.Err(), "datastore sync stopped")
			}
			return
		case syncup := <-d.synCh:
			switch {
			case syncup.Start:
				log.V(logf.VDebug).Info("sync start")
				startTs = time.Now().Unix()

			case syncup.End:
				log.V(logf.VDebug).Info("sync end")

				startTs = 0

				d.syncTreeMutex.Lock()
				d.syncTree = d.syncTreeCandidate
				d.syncTreeMutex.Unlock()

				// create new syncTreeCandidat
				d.syncTreeCandidate, err = tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, tree.RunningIntentName))
				if err != nil {
					log.Error(err, "failed creating a new synctree candidate")
					return
				}

				// export and write to cache
				runningExport, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio)
				if err != nil {
					log.Error(err, "failed exporting tree")
					continue
				}
				err = d.cacheClient.IntentModify(ctx, runningExport)
				if err != nil {
					log.Error(err, "failed modifying running cache content")
					continue
				}
			default:
				if startTs == 0 {
					startTs = time.Now().Unix()
				}
				err := d.writeToSyncTreeCandidate(ctx, syncup.Update.GetUpdate(), startTs)
				if err != nil {
					log.Error(err, "failed to write to sync tree")
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

	// fmt.Println(upds.String())
	for idx, upd := range upds {
		_ = idx
		_, err := d.syncTreeCandidate.AddUpdateRecursive(ctx, upd, treetypes.NewUpdateInsertFlags())
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

	d.deviationClients[pName] = stream
	return nil
}

func (d *Datastore) StopDeviationsWatch(peer string) {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.deviationClients, peer)
	logf.DefaultLogger.V(1).Info("deviation watcher removed", "peer", peer)
}

func (d *Datastore) DeviationMgr(ctx context.Context) {
	log := logf.FromContext(ctx).WithName("deviation-mgr")
	ctx = logf.IntoContext(ctx, log)
	log.Info("starting deviationMgr")
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
			deviationClientNames := make([]string, 0, len(d.deviationClients))
			deviationClients := map[string]sdcpb.DataServer_WatchDeviationsServer{}
			for clientIdentifier, devStream := range d.deviationClients {
				deviationClients[clientIdentifier] = devStream
				deviationClientNames = append(deviationClientNames, clientIdentifier)
			}
			d.m.RUnlock()
			if len(deviationClients) == 0 {
				log.V(logf.VDebug).Info("no deviation clients present")
				continue
			}
			log.V(logf.VDebug).Info("got deviations clients", "deviation-clients", strings.Join(deviationClientNames, ", "))
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_START,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
			deviationChan, err := d.calculateDeviations(ctx)
			if err != nil {
				log.Error(err, "error calculating deviations")
				continue
			}
			d.SendDeviations(deviationChan, deviationClients)
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_END,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
		}
	}
}

func (d *Datastore) SendDeviations(ch <-chan *treetypes.DeviationEntry, deviationClients map[string]sdcpb.DataServer_WatchDeviationsServer) {
	wg := &sync.WaitGroup{}
	for {
		select {
		case de, ok := <-ch:
			if !ok {
				wg.Wait()
				return
			}
			wg.Add(1)
			go func(de DeviationEntry, dcs map[string]sdcpb.DataServer_WatchDeviationsServer) {
				for clientIdentifier, dc := range dcs {
					ctx := dc.Context()
					log := logf.FromContext(ctx)
					err := dc.Send(&sdcpb.WatchDeviationResponse{
						Name:          d.config.Name,
						Intent:        de.IntentName(),
						Event:         sdcpb.DeviationEvent_UPDATE,
						Reason:        sdcpb.DeviationReason(de.Reason()),
						Path:          de.Path(),
						ExpectedValue: de.ExpectedValue(),
						CurrentValue:  de.CurrentValue(),
					})
					if err != nil {
						log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
					}
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
		deviationTree.GetDeviations(ctx, deviationChan)
		close(deviationChan)
	}()

	return deviationChan, nil
}

func (d *Datastore) BlameConfig(ctx context.Context, includeDefaults bool) (*sdcpb.BlameTreeElement, error) {
	// create a new TreeRoot by copying the syncTree
	d.syncTreeMutex.Lock()
	root, err := d.syncTree.DeepCopy(ctx)
	d.syncTreeMutex.Unlock()
	if err != nil {
		return nil, err
	}
	// load all intents
	_, err = d.LoadAllButRunningIntents(ctx, root)
	if err != nil {
		return nil, err
	}
	// calculate the Blame
	bte, err := root.BlameConfig(includeDefaults)
	if err != nil {
		return nil, err
	}
	// set the root level elements name to the target name
	bte.Name = d.config.Name
	return bte, nil
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
