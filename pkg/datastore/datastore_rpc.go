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
	"runtime"
	"sync"
	"time"

	"github.com/sdcio/logger"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree"
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

	ctx context.Context
	// stop cancel func
	cfn context.CancelFunc

	// keeps track of clients watching deviation updates
	m                *sync.RWMutex
	deviationClients map[sdcpb.DataServer_WatchDeviationsServer]string

	// datastore mutex locks the whole datasore for further set operations
	dmutex *sync.Mutex

	// TransactionManager
	transactionManager *types.TransactionManager

	// SyncTree
	syncTree      *tree.RootEntry
	syncTreeMutex *sync.RWMutex

	taskPool *pool.SharedTaskPool
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(ctx context.Context, c *config.DatastoreConfig, sc schema.Client, cc cache.Client, opts ...grpc.DialOption) (*Datastore, error) {

	ctx, cancel := context.WithCancel(ctx)

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
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	syncTreeRoot, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		cancel()
		return nil, err
	}

	ccb := cache.NewCacheClientBound(c.Name, cc)

	ds := &Datastore{
		config:           c,
		schemaClient:     scb,
		ctx:              ctx,
		cfn:              cancel,
		cacheClient:      ccb,
		m:                &sync.RWMutex{},
		dmutex:           &sync.Mutex{},
		deviationClients: make(map[sdcpb.DataServer_WatchDeviationsServer]string),
		syncTree:         syncTreeRoot,
		syncTreeMutex:    &sync.RWMutex{},
		taskPool:         pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
	}
	ds.transactionManager = types.NewTransactionManager(NewDatastoreRollbackAdapter(ds))

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
			cancel()
			return
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
	d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, d, d.config.Sync.Config, d.taskPool, opts...)
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
			d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, d, d.config.Sync.Config, d.taskPool, opts...)
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

func (d *Datastore) ConnectionState() *targettypes.TargetStatus {
	if d.sbi == nil {
		return targettypes.NewTargetStatus(targettypes.TargetStatusNotConnected)
	}
	return d.sbi.Status()
}

func (d *Datastore) Stop(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.cfn()
	if d.sbi == nil {
		return nil
	}
	err := d.sbi.Close(ctx)
	if err != nil {
		logf.DefaultLogger.Error(err, "datastore failed to close the target connection", "datastore-name", d.Name())
	}
	return nil
}

func (d *Datastore) BlameConfig(ctx context.Context, includeDefaults bool) (*sdcpb.BlameTreeElement, error) {
	log := logger.FromContext(ctx).WithName("BlameConfig")
	ctx = logger.IntoContext(ctx, log)

	// create a new TreeRoot by copying the syncTree
	d.syncTreeMutex.RLock()
	root, err := d.syncTree.DeepCopy(ctx)
	d.syncTreeMutex.RUnlock()
	if err != nil {
		return nil, err
	}
	// load all intents
	_, err = d.LoadAllButRunningIntents(ctx, root)
	if err != nil {
		return nil, err
	}

	blamePool := d.taskPool.NewVirtualPool(pool.VirtualFailFast)
	bcp := tree.NewBlameConfigProcessor(tree.NewBlameConfigProcessorConfig(includeDefaults))

	bte, err := bcp.Run(ctx, root.GetRoot(), blamePool)

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
