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
	"errors"
	"fmt"
	"strings"
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

	// dm guards driver, reconnecting, and nextCapsReconnect. driver is nil while disconnected.
	dm                sync.RWMutex
	driver            Driver
	reconnecting      bool
	nextCapsReconnect time.Time

	// capsReconnectCooldown overrides defaultCapabilitiesReconnectCooldown when positive.
	capsReconnectCooldown time.Duration

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

// NewNCTarget creates a NETCONF target whose background reconnect loop is
// bound to ctx and to Close, whichever ends first. A nil ctx means the loop
// lives until Close.
func NewNCTarget(ctx context.Context, name string, cfg *config.SBI, runningStore types.RunningStore, schemaClient schemaClient.SchemaClientBound) (*ncTarget, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lifeCtx, lifeStop := context.WithCancel(ctx)
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
		lifeStop()
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
		if sessErr, ok := t.handleSessionError(ctx, drv, err); ok {
			return nil, sessErr
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
	case config.NCCommitDatastoreRunning, config.NCCommitDatastoreCandidate:
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

// retireDriver detaches failed from the target if it is still the active
// driver, starts a background reconnect, and closes failed asynchronously so
// the caller never blocks on a dead or wedged peer.
func (t *ncTarget) retireDriver(ctx context.Context, failed Driver) {
	t.dm.Lock()
	if failed != nil && t.driver == failed {
		t.driver = nil
		if !t.reconnecting {
			t.reconnecting = true
			go t.reconnect(logf.IntoContext(t.reconnectCtx(), logf.FromContext(ctx)), 0)
		}
	}
	t.dm.Unlock()

	if failed != nil {
		go func() { _ = failed.Close() }()
	}
}

// retirePoisoned retires failed when candidate cleanup or unlock fails, leaving
// the primary RPC result untouched while respecting capabilities-changed cooldown.
func (t *ncTarget) retirePoisoned(ctx context.Context, failed Driver, err error) {
	if errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		t.retireCapabilitiesChanged(ctx, failed, err)
		return
	}
	t.retireDriver(ctx, failed)
}

// handleTransportError retires the driver that failed, starts a background
// reconnect, and wraps err in types.ErrNotConnected so the caller's caller
// (ultimately the TransactionSet gRPC handler, via translateInternalToGrpcError)
// can classify it as recoverable.
func (t *ncTarget) handleTransportError(ctx context.Context, failed Driver, err error) error {
	t.retireDriver(ctx, failed)
	return fmt.Errorf("%s: %w: %w", t.name, types.ErrNotConnected, err)
}

// handleSessionError retires drv and wraps err in ErrNotConnected when err
// indicates a dead or stale session (transport drop or capabilities-changed).
// It reports false without side effects for semantic RPC errors.
func (t *ncTarget) handleSessionError(ctx context.Context, drv Driver, err error) (error, bool) {
	if isTransportError(err) {
		return t.handleTransportError(ctx, drv, err), true
	}
	if errors.Is(err, nctypes.ErrCapabilitiesChanged) {
		return t.handleCapabilitiesChanged(ctx, drv, err), true
	}
	return err, false
}

// defaultCapabilitiesReconnectCooldown is the minimum interval between two
// reconnect dials triggered by a capabilities-changed rpc-error.
const defaultCapabilitiesReconnectCooldown = 30 * time.Second

func (t *ncTarget) capabilitiesCooldown() time.Duration {
	if t.capsReconnectCooldown > 0 {
		return t.capsReconnectCooldown
	}
	return defaultCapabilitiesReconnectCooldown
}

func (t *ncTarget) retireCapabilitiesChanged(ctx context.Context, failed Driver, err error) {
	log := logf.FromContext(ctx)
	cooldown := t.capabilitiesCooldown()

	var (
		retired bool
		delay   time.Duration
	)
	t.dm.Lock()
	if failed != nil && t.driver == failed {
		retired = true
		t.driver = nil
		now := time.Now()
		if now.Before(t.nextCapsReconnect) {
			delay = t.nextCapsReconnect.Sub(now)
		}
		t.nextCapsReconnect = now.Add(delay).Add(cooldown)
		if !t.reconnecting {
			t.reconnecting = true
			go t.reconnect(logf.IntoContext(t.reconnectCtx(), log), delay)
		}
	}
	t.dm.Unlock()

	if retired {
		if delay > 0 {
			log.Info("device reports capabilities-changed within cooldown, delaying reconnect", "delay", delay, "error", err.Error())
		} else {
			log.Info("device reports capabilities-changed, reconnecting", "error", err.Error())
		}
	}
	if failed != nil {
		go func() { _ = failed.Close() }()
	}
}

// handleCapabilitiesChanged retires failed after a capabilities-changed
// rpc-error and schedules a background reconnect so the new session performs a
// fresh hello exchange.
//
// Dials triggered by capabilities-changed are spaced by at least the target's
// capabilities cooldown; if another session hits capabilities-changed inside
// that window, it is still retired immediately while dialing waits out the
// remaining cooldown.
func (t *ncTarget) handleCapabilitiesChanged(ctx context.Context, failed Driver, err error) error {
	t.retireCapabilitiesChanged(ctx, failed, err)
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

// reconnect opens a replacement driver, waiting initialDelay before the first
// attempt and retrying every ConnectRetry until one succeeds or ctx ends. Only
// one reconnect runs at a time (see reconnecting).
func (t *ncTarget) reconnect(ctx context.Context, initialDelay time.Duration) {
	log := logf.FromContext(ctx)
	newDriver := t.newDriver
	if newDriver == nil {
		newDriver = newScrapligoDriver
	}

	if initialDelay > 0 {
		timer := time.NewTimer(initialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.finishReconnect(ctx, nil)
			return
		case <-timer.C:
		}
	}

	log.Info("NETCONF reconnecting")
	for {
		d, err := newDriver(t.sbiConfig)
		if err == nil {
			if !t.finishReconnect(ctx, d) {
				_ = d.Close()
				return
			}
			log.Info("NETCONF reconnected")
			return
		}
		log.Error(err, "failed to create NETCONF driver")

		timer := time.NewTimer(t.sbiConfig.ConnectRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.finishReconnect(ctx, nil)
			return
		case <-timer.C:
		}
	}
}

// finishReconnect ends the reconnect cycle and installs d as the current
// driver. It reports false without installing d when ctx is already done or
// d is nil; the caller then owns closing d.
func (t *ncTarget) finishReconnect(ctx context.Context, d Driver) bool {
	t.dm.Lock()
	defer t.dm.Unlock()
	t.reconnecting = false
	if d == nil || ctx.Err() != nil {
		return false
	}
	t.driver = d
	return true
}

// filterRPCErrors takes the given etree.Document, filters the document for rpc-errors with the given severity
// and returns them collectively as a []string. The severity is compared after trimming whitespace, matching
// how the driver classifies rpc-errors, and the document is left unmodified.
func filterRPCErrors(xml *etree.Document, severity string) ([]string, error) {
	var result []string
	for _, rpcErr := range xml.FindElements("//rpc-error") {
		sev := rpcErr.SelectElement("error-severity")
		if sev == nil || strings.TrimSpace(sev.Text()) != severity {
			continue
		}
		d := etree.NewDocumentWithRoot(rpcErr.Copy())
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
	ctx = logf.IntoContext(ctx, log)
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

	if commitDatastore == config.NCCommitDatastoreCandidate {
		return t.setCandidate(ctx, drv, xdoc)
	}
	return t.setRunning(ctx, drv, xdoc)
}

func (t *ncTarget) setRunning(ctx context.Context, drv Driver, xdoc string) (*sdcpb.SetDataResponse, error) {
	resp, err := drv.EditConfig(config.NCCommitDatastoreRunning, xdoc)
	if err != nil {
		logf.FromContext(ctx).Error(err, "failed during edit-config")
		if sessErr, ok := t.handleSessionError(ctx, drv, err); ok {
			return nil, sessErr
		}
		return nil, err
	}
	rpcWarnings, err := filterRPCErrors(resp.Doc, "warning")
	if err != nil {
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}
	return &sdcpb.SetDataResponse{
		Warnings:  rpcWarnings,
		Timestamp: time.Now().UnixNano(),
	}, nil
}

func (t *ncTarget) setCandidate(ctx context.Context, drv Driver, xdoc string) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx)
	if err := t.lockCandidate(ctx, drv); err != nil {
		return nil, err
	}

	unlockCandidate := func() {
		if _, err := drv.Unlock(config.NCCommitDatastoreCandidate); err != nil {
			log.Error(err, "failed unlocking candidate datastore, retiring session")
			t.retirePoisoned(ctx, drv, err)
		}
	}

	cleanupFailedCandidate := func() {
		if err := drv.Discard(); err != nil {
			log.Error(err, "failed while discarding pending changes, retiring session")
			t.retirePoisoned(ctx, drv, err)
			return
		}
		unlockCandidate()
	}

	resp, err := drv.EditConfig(config.NCCommitDatastoreCandidate, xdoc)
	if err != nil {
		log.Error(err, "failed during edit-config")
		if sessErr, ok := t.handleSessionError(ctx, drv, err); ok {
			return nil, sessErr
		}
		cleanupFailedCandidate()
		return nil, err
	}
	rpcWarnings, err := filterRPCErrors(resp.Doc, "warning")
	if err != nil {
		cleanupFailedCandidate()
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}

	log.Info("committing changes on target")
	commitResp, err := drv.Commit()
	if err != nil {
		log.Error(err, "failed during commit")
		if sessErr, ok := t.handleSessionError(ctx, drv, err); ok {
			return nil, sessErr
		}
		cleanupFailedCandidate()
		return nil, err
	}
	commitWarnings, err := filterRPCErrors(commitResp.Doc, "warning")
	if err != nil {
		unlockCandidate()
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}
	rpcWarnings = append(rpcWarnings, commitWarnings...)
	unlockCandidate()
	return &sdcpb.SetDataResponse{
		Warnings:  rpcWarnings,
		Timestamp: time.Now().UnixNano(),
	}, nil
}

// lockCandidate acquires the candidate datastore lock, discarding uncommitted
// candidate changes and retrying once when lock failure reports no active lock holder.
func (t *ncTarget) lockCandidate(ctx context.Context, drv Driver) error {
	log := logf.FromContext(ctx)
	_, err := drv.Lock(config.NCCommitDatastoreCandidate)
	if err == nil {
		return nil
	}
	if sessErr, ok := t.handleSessionError(ctx, drv, err); ok {
		return sessErr
	}
	if errors.Is(err, nctypes.ErrLockHeld) {
		log.Error(err, "failed locking candidate datastore")
		return err
	}

	log.Info("candidate lock failed without active lock holder, discarding pending changes and retrying lock", "error", err.Error())
	if discardErr := drv.Discard(); discardErr != nil {
		if sessErr, ok := t.handleSessionError(ctx, drv, discardErr); ok {
			return sessErr
		}
		log.Error(discardErr, "failed discarding dirty candidate before lock retry")
		return err
	}

	if _, retryErr := drv.Lock(config.NCCommitDatastoreCandidate); retryErr != nil {
		log.Error(retryErr, "failed locking candidate datastore after discard")
		if sessErr, ok := t.handleSessionError(ctx, drv, retryErr); ok {
			return sessErr
		}
		return retryErr
	}
	return nil
}
