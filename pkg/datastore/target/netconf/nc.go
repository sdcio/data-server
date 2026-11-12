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

package netconf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/beevik/etree"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/netconf/driver/scrapligo"
	nctypes "github.com/sdcio/data-server/pkg/datastore/target/netconf/types"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/importer/xml"
)

type ncTarget struct {
	name string

	// dm guards driver and reconnecting. driver is nil while disconnected.
	dm           sync.RWMutex
	driver       Driver
	reconnecting bool

	// lifeCtx outlives individual requests and is cancelled by Close; it
	// bounds the background reconnect loop. A nil lifeCtx means no bound.
	lifeCtx   context.Context
	lifeStop  context.CancelFunc
	newDriver func(*config.SBI) (Driver, error)

	syncs            map[string]NetconfSync
	schemaClient     schemaClient.SchemaClientBound
	sbiConfig        *config.SBI
	xml2sdcpbAdapter *XML2sdcpbConfigAdapter
	runningStore     types.RunningStore
}

func NewNCTarget(_ context.Context, name string, cfg *config.SBI, runningStore types.RunningStore, schemaClient schemaClient.SchemaClientBound) (*ncTarget, error) {
	lifeCtx, lifeStop := context.WithCancel(context.Background())
	t := &ncTarget{
		name:             name,
		lifeCtx:          lifeCtx,
		lifeStop:         lifeStop,
		newDriver:        newScrapligoDriver,
		schemaClient:     schemaClient,
		sbiConfig:        cfg,
		xml2sdcpbAdapter: NewXML2sdcpbConfigAdapter(schemaClient),
		syncs:            map[string]NetconfSync{},
		runningStore:     runningStore,
	}
	// create a new NETCONF driver
	d, err := t.newDriver(cfg)
	if err != nil {
		return t, err
	}
	t.driver = d
	return t, nil
}

func newScrapligoDriver(cfg *config.SBI) (Driver, error) {
	d, err := scrapligo.NewScrapligoNetconfTarget(cfg)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// getDriver returns the current driver, or an ErrNotConnected error while
// a reconnect is pending.
func (t *ncTarget) getDriver() (Driver, error) {
	t.dm.RLock()
	defer t.dm.RUnlock()
	if t.driver == nil {
		return nil, fmt.Errorf("%s: %w", t.name, types.ErrNotConnected)
	}
	return t.driver, nil
}

func (t *ncTarget) AddSyncs(ctx context.Context, sps ...*config.SyncProtocol) error {
	for _, sp := range sps {
		ncSync, err := NewNetconfSyncImpl(ctx, t.name, t, sp, t.runningStore)
		if err != nil {
			return err
		}
		t.syncs[sp.Name] = ncSync

		err = ncSync.Start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *ncTarget) GetImportAdapter(ctx context.Context, req *sdcpb.GetDataRequest) (importer.ImportConfigAdapter, error) {
	ncResponse, err := t.internalGet(ctx, req)
	if err != nil {
		return nil, err
	}

	xmlImport := xml.NewXmlTreeImporter(ncResponse.Doc.Root(), consts.RunningIntentName, consts.RunningValuesPrio, false)

	return xmlImport, nil
}

func (t *ncTarget) internalGet(ctx context.Context, req *sdcpb.GetDataRequest) (*nctypes.NetconfResponse, error) {
	log := logf.FromContext(ctx).WithName("Get")
	ctx = logf.IntoContext(ctx, log)

	if err := t.Status().Err(); err != nil {
		return nil, err
	}
	source := "running"

	// init a new XMLConfigBuilder for the pathfilter
	pathfilterXmlBuilder := NewXMLConfigBuilder(t.schemaClient,
		&XMLConfigBuilderOpts{
			HonorNamespace:         t.sbiConfig.NetconfOptions.IncludeNS,
			OperationWithNamespace: t.sbiConfig.NetconfOptions.OperationWithNamespace,
			UseOperationRemove:     t.sbiConfig.NetconfOptions.UseOperationRemove,
		})

	// add all the requested paths to the document
	for _, p := range req.Path {
		err := pathfilterXmlBuilder.AddElements(ctx, p)
		if err != nil {
			return nil, err
		}
	}

	// retrieve the xml filter as string
	filterDoc, err := pathfilterXmlBuilder.GetDoc()
	if err != nil {
		return nil, err
	}

	log.V(logf.VDebug).Info("netconf get", "filter", filterDoc, "source", source)

	drv, err := t.getDriver()
	if err != nil {
		return nil, err
	}

	// execute the GetConfig rpc
	ncResponse, err := drv.GetConfig(source, filterDoc)
	if err != nil {
		if isTransportError(err) {
			return nil, t.handleTransportError(ctx, drv, err)
		}
		return nil, err
	}
	log.V(logf.VTrace).Info("received netconf response", "response", ncResponse.DocAsString(false))
	return ncResponse, err
}

func (t *ncTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	ncResponse, err := t.internalGet(ctx, req)
	if err != nil {
		return nil, err
	}

	// start transformation, which yields the sdcpb_Notification
	noti, err := t.xml2sdcpbAdapter.Transform(ctx, ncResponse.Doc)
	if err != nil {
		return nil, err
	}

	// building the resulting sdcpb.GetDataResponse struct
	result := &sdcpb.GetDataResponse{
		Notification: noti,
	}
	return result, nil
}

func (t *ncTarget) Set(ctx context.Context, source types.TargetSource) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx).WithName("Set")
	ctx = logf.IntoContext(ctx, log)
	if err := t.Status().Err(); err != nil {
		return nil, err
	}

	switch t.sbiConfig.NetconfOptions.CommitDatastore {
	case "running", "candidate":
		return t.setToDevice(ctx, t.sbiConfig.NetconfOptions.CommitDatastore, source)
	}
	// should not get here if the config validation happened.
	return nil, fmt.Errorf("unknown commit-datastore: %s", t.sbiConfig.NetconfOptions.CommitDatastore)
}

func (t *ncTarget) Status() *types.TargetStatus {
	result := types.NewTargetStatus(sdcpb.TargetStatus_NOT_CONNECTED)
	if t == nil {
		result.Details = "connection not initialized"
		return result
	}
	t.dm.RLock()
	drv := t.driver
	t.dm.RUnlock()
	if drv == nil {
		result.Details = "connection not initialized"
		return result
	}
	if drv.IsAlive() {
		result.Status = sdcpb.TargetStatus_CONNECTED
	}
	return result
}

// handleTransportError retires the driver that failed, starts a background
// reconnect, and wraps err in types.ErrNotConnected so the caller's caller
// (ultimately the TransactionSet gRPC handler, via translateInternalToGrpcError)
// can classify it as recoverable.
//
// failed is the driver the failing RPC ran on. If it was already replaced by
// a concurrent failure or a reconnect, the current driver is left untouched.
// The old driver is closed in the background so the caller never waits on a
// dead peer.
func (t *ncTarget) handleTransportError(ctx context.Context, failed Driver, err error) error {
	t.dm.Lock()
	if t.driver == failed {
		t.driver = nil
		if !t.reconnecting {
			t.reconnecting = true
			go t.reconnect(logf.IntoContext(t.reconnectCtx(), logf.FromContext(ctx)))
		}
	}
	t.dm.Unlock()

	go func() { _ = failed.Close() }()
	return fmt.Errorf("%s: %w: %w", t.name, types.ErrNotConnected, err)
}

func (t *ncTarget) reconnectCtx() context.Context {
	if t.lifeCtx == nil {
		return context.Background()
	}
	return t.lifeCtx
}

// Close stops any pending reconnect and closes the current driver.
func (t *ncTarget) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if t.lifeStop != nil {
		t.lifeStop()
	}
	t.dm.Lock()
	drv := t.driver
	t.driver = nil
	t.dm.Unlock()
	if drv == nil {
		return nil
	}
	return drv.Close()
}

// reconnect retries opening a driver every ConnectRetry until it succeeds or
// ctx is cancelled. Only one reconnect runs at a time (see reconnecting).
func (t *ncTarget) reconnect(ctx context.Context) {
	log := logf.FromContext(ctx)
	newDriver := t.newDriver
	if newDriver == nil {
		newDriver = newScrapligoDriver
	}

	log.Info("NETCONF reconnecting")
	for {
		d, err := newDriver(t.sbiConfig)
		if err == nil {
			t.dm.Lock()
			t.reconnecting = false
			if ctx.Err() != nil {
				t.dm.Unlock()
				_ = d.Close()
				return
			}
			t.driver = d
			t.dm.Unlock()
			log.Info("NETCONF reconnected")
			return
		}
		log.Error(err, "failed to create NETCONF driver")

		timer := time.NewTimer(t.sbiConfig.ConnectRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.dm.Lock()
			t.reconnecting = false
			t.dm.Unlock()
			return
		case <-timer.C:
		}
	}
}

// filterRPCErrors takes the given etree.Document, filters the document for rpc-errors with the given severity
// and returns them collectively as a []string
func filterRPCErrors(xml *etree.Document, severity string) ([]string, error) {
	var result []string
	rpcErrs := xml.FindElements(fmt.Sprintf("//rpc-error[error-severity='%s']", severity))
	for _, rpcErr := range rpcErrs {
		d := etree.NewDocumentWithRoot(rpcErr)
		s, err := d.WriteToString()
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, nil
}

func (t *ncTarget) setToDevice(ctx context.Context, commitDatastore string, source types.TargetSource) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx).WithValues("commit-datastore", commitDatastore)
	xtree, err := source.ToXML(ctx, true, t.sbiConfig.NetconfOptions.IncludeNS, t.sbiConfig.NetconfOptions.OperationWithNamespace, t.sbiConfig.NetconfOptions.UseOperationRemove)
	if err != nil {
		return nil, err
	}

	xdoc, err := xtree.WriteToString()
	if err != nil {
		return nil, err
	}

	// if there was no data in the xml document, continue
	if len(xdoc) == 0 {
		return &sdcpb.SetDataResponse{
			Timestamp: time.Now().UnixNano(),
		}, nil
	}

	log.V(logf.VTrace).Info("generated config XML", "xml", xdoc)

	drv, err := t.getDriver()
	if err != nil {
		return nil, err
	}

	// edit the config
	resp, err := drv.EditConfig(commitDatastore, xdoc)
	if err != nil {
		log.Error(err, "failed during edit-config")
		if isTransportError(err) {
			return nil, t.handleTransportError(ctx, drv, err)
		}

		// candidate should discard on error
		if commitDatastore == "candidate" {
			err2 := drv.Discard()
			if err2 != nil {
				// log failed discard
				log.Error(err2, "failed while discarding pending changes")
			}
		}
		return nil, err
	}
	rpcWarnings, err := filterRPCErrors(resp.Doc, "warning")
	if err != nil {
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}

	// candidate stores need to commit the changes to running
	if commitDatastore == "candidate" {
		log.Info("committing changes on target")
		// commit the config
		err = drv.Commit()
		if err != nil {
			if isTransportError(err) {
				return nil, t.handleTransportError(ctx, drv, err)
			}
			return nil, err
		}
	}
	return &sdcpb.SetDataResponse{
		Warnings:  rpcWarnings,
		Timestamp: time.Now().UnixNano(),
	}, nil
}
