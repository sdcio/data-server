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

	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"

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
	)
	ctx = logf.IntoContext(ctx, log)

	log.Info("new datastore",
		"target-name", c.Name,
		"schema-vendor", c.Schema.Vendor,
		"schema-version", c.Schema.Version,
		"sbi-type", c.SBI.Type,
		"sbi-address", c.SBI.Address,
		"sbi-port", c.SBI.Port,
	)

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
		ds.DeviationMgr(ctx, c.Deviation)
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
		logf.DefaultLogger.Error(err, "datastore failed to close the target connection", "datastore-name", d.Name())
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
				runningExport, err := d.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
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

		_, err := d.syncTreeCandidate.AddUpdateRecursive(ctx, upd.GetPath(), upd.GetUpdate(), treetypes.NewUpdateInsertFlags())
		if err != nil {
			return err
		}
	}
	return nil
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
	_, err = d.LoadAllButRunningIntents(ctx, root, true)
	if err != nil {
		return nil, err
	}

	// calculate the Blame
	bcv := tree.NewBlameConfigVisitor(includeDefaults)
	err = root.Walk(ctx, bcv)
	if err != nil {
		return nil, err
	}
	bte := bcv.GetResult()

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
