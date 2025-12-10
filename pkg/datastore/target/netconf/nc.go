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
	"strings"
	"sync"
	"time"

	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/netconf/driver/scrapligo"
	nctypes "github.com/sdcio/data-server/pkg/datastore/target/netconf/types"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/importer/xml"
)

type ncTarget struct {
	name   string
	driver Driver

	m *sync.Mutex

	syncs            map[string]NetconfSync
	schemaClient     schemaClient.SchemaClientBound
	sbiConfig        *config.SBI
	xml2sdcpbAdapter *XML2sdcpbConfigAdapter
	runningStore     types.RunningStore
}

func NewNCTarget(_ context.Context, name string, cfg *config.SBI, runningStore types.RunningStore, schemaClient schemaClient.SchemaClientBound) (*ncTarget, error) {
	t := &ncTarget{
		name:             name,
		m:                new(sync.Mutex),
		schemaClient:     schemaClient,
		sbiConfig:        cfg,
		xml2sdcpbAdapter: NewXML2sdcpbConfigAdapter(schemaClient),
		syncs:            map[string]NetconfSync{},
		runningStore:     runningStore,
	}
	var err error
	// create a new NETCONF driver
	t.driver, err = scrapligo.NewScrapligoNetconfTarget(cfg)
	if err != nil {
		return t, err
	}
	return t, nil
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

	cmlImport := xml.NewXmlTreeImporter(ncResponse.Doc.Root())

	return cmlImport, nil
}

func (t *ncTarget) internalGet(ctx context.Context, req *sdcpb.GetDataRequest) (*nctypes.NetconfResponse, error) {
	if !t.Status().IsConnected() {
		return nil, fmt.Errorf("%s", types.TargetStatusNotConnected)
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
	log.Debugf("netconf filter:\n%s", filterDoc)

	// execute the GetConfig rpc
	ncResponse, err := t.driver.GetConfig(source, filterDoc)
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			t.Close(ctx)
			go t.reconnect()
		}
		return nil, err
	}
	log.Debugf("%s: netconf response:\n%s", t.name, ncResponse.DocAsString())
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
	if !t.Status().IsConnected() {
		return nil, fmt.Errorf("%s", types.TargetStatusNotConnected)
	}

	switch t.sbiConfig.NetconfOptions.CommitDatastore {
	case "running":
		return t.setRunning(ctx, source)
	case "candidate":
		return t.setCandidate(ctx, source)
	}
	// should not get here if the config validation happened.
	return nil, fmt.Errorf("unknown commit-datastore: %s", t.sbiConfig.NetconfOptions.CommitDatastore)
}

func (t *ncTarget) Status() *types.TargetStatus {
	result := types.NewTargetStatus(types.TargetStatusNotConnected)
	if t == nil || t.driver == nil {
		result.Details = "connection not initialized"
		return result
	}
	if t.driver.IsAlive() {
		result.Status = types.TargetStatusConnected
	}
	return result
}

func (t *ncTarget) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if t.driver == nil {
		return nil
	}
	return t.driver.Close()
}

func (t *ncTarget) reconnect() {
	t.m.Lock()
	defer t.m.Unlock()

	if t.Status().IsConnected() {
		return
	}

	var err error
	log.Infof("%s: NETCONF reconnecting...", t.name)
	for {
		t.driver, err = scrapligo.NewScrapligoNetconfTarget(t.sbiConfig)
		if err != nil {
			log.Errorf("failed to create NETCONF driver: %v", err)
			time.Sleep(t.sbiConfig.ConnectRetry)
			continue
		}
		log.Infof("%s: NETCONF reconnected...", t.name)
		return
	}
}

func (t *ncTarget) setRunning(ctx context.Context, source types.TargetSource) (*sdcpb.SetDataResponse, error) {

	xtree, err := source.ToXML(true, t.sbiConfig.NetconfOptions.IncludeNS, t.sbiConfig.NetconfOptions.OperationWithNamespace, t.sbiConfig.NetconfOptions.UseOperationRemove)
	if err != nil {
		return nil, err
	}

	xdoc, err := xtree.WriteToString()
	if err != nil {
		return nil, err
	}

	// if there was no data in the xml document, return
	if len(xdoc) == 0 {
		return &sdcpb.SetDataResponse{
			Timestamp: time.Now().UnixNano(),
		}, nil
	}

	log.Debugf("datastore %s XML:\n%s\n", t.name, xdoc)

	// edit the config
	resp, err := t.driver.EditConfig("running", xdoc)
	if err != nil {
		log.Errorf("datastore %s failed edit-config: %v", t.name, err)
		if strings.Contains(err.Error(), "EOF") {
			t.Close(ctx)
			go t.reconnect()
			return nil, err
		}
		return nil, err
	}

	// retrieve netconf rpc-error -> warnings as string array
	warnings, err := filterRPCErrors(resp.Doc, "warning")
	if err != nil {
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}
	return &sdcpb.SetDataResponse{
		Warnings:  warnings,
		Timestamp: time.Now().UnixNano(),
	}, nil
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

func (t *ncTarget) setCandidate(ctx context.Context, source types.TargetSource) (*sdcpb.SetDataResponse, error) {
	xtree, err := source.ToXML(true, t.sbiConfig.NetconfOptions.IncludeNS, t.sbiConfig.NetconfOptions.OperationWithNamespace, t.sbiConfig.NetconfOptions.UseOperationRemove)
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

	log.Debugf("datastore %s XML:\n%s\n", t.name, xdoc)

	// edit the config
	resp, err := t.driver.EditConfig("candidate", xdoc)
	if err != nil {
		log.Errorf("datastore %s failed edit-config: %v", t.name, err)
		if strings.Contains(err.Error(), "EOF") {
			t.Close(ctx)
			go t.reconnect()
			return nil, err
		}
		err2 := t.driver.Discard()
		if err2 != nil {
			// log failed discard
			log.Errorf("failed with %v while discarding pending changes after error %v", err2, err)
		}
		return nil, err
	}
	rpcWarnings, err := filterRPCErrors(resp.Doc, "warning")
	if err != nil {
		return nil, fmt.Errorf("filtering netconf rpc-errors with severity warnings: %w", err)
	}

	log.Infof("datastore %s: committing changes on target", t.name)
	// commit the config
	err = t.driver.Commit()
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			t.Close(ctx)
			go t.reconnect()
		}
		return nil, err
	}
	return &sdcpb.SetDataResponse{
		Warnings:  rpcWarnings,
		Timestamp: time.Now().UnixNano(),
	}, nil
}
