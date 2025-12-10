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

package gnmi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/openconfig/gnmi/proto/gnmi"
	gtarget "github.com/openconfig/gnmic/pkg/api/target"
	"github.com/openconfig/gnmic/pkg/api/types"
	"github.com/sdcio/data-server/pkg/config"
	gnmiutils "github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	targetTypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/utils"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

const (
	syncRetryWaitTime = 10 * time.Second
)

type gnmiTarget struct {
	target          *gtarget.Target
	encodings       map[gnmi.Encoding]struct{}
	cfg             *config.SBI
	syncs           map[string]GnmiSync
	runningStore    targetTypes.RunningStore
	schemaClient    dsutils.SchemaClientBound
	taskpoolFactory pool.VirtualPoolFactory
}

func NewTarget(ctx context.Context, name string, cfg *config.SBI, runningStore targetTypes.RunningStore, schemaClient dsutils.SchemaClientBound, taskpoolFactory pool.VirtualPoolFactory, opts ...grpc.DialOption) (*gnmiTarget, error) {
	tc := &types.TargetConfig{
		Name:       name,
		Address:    fmt.Sprintf("%s:%d", cfg.Address, cfg.Port),
		Timeout:    10 * time.Second,
		RetryTimer: 2 * time.Second,
		BufferSize: 100,
	}
	if cfg.Credentials != nil {
		tc.Username = &cfg.Credentials.Username
		tc.Password = &cfg.Credentials.Password
	}
	if cfg.TLS != nil {
		tc.TLSCA = &cfg.TLS.CA
		tc.TLSCert = &cfg.TLS.Cert
		tc.TLSKey = &cfg.TLS.Key
		tc.SkipVerify = &cfg.TLS.SkipVerify
	} else {
		tc.Insecure = pointer.ToBool(true)
	}
	gt := &gnmiTarget{
		target:          gtarget.NewTarget(tc),
		encodings:       make(map[gnmi.Encoding]struct{}),
		cfg:             cfg,
		syncs:           map[string]GnmiSync{},
		runningStore:    runningStore,
		schemaClient:    schemaClient,
		taskpoolFactory: taskpoolFactory,
	}

	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                10 * time.Second, // send pings every 10s if no activity
		Timeout:             2 * time.Second,  // wait 2s for ping ack before considering dead
		PermitWithoutStream: true,             // send pings even without active RPCs
	}))

	err := gt.target.CreateGNMIClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	// discover supported encodings
	capResp, err := gt.target.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	for _, enc := range capResp.GetSupportedEncodings() {
		gt.encodings[enc] = struct{}{}
	}

	if _, exists := gt.encodings[gnmi.Encoding(gnmiutils.ParseGnmiEncoding(cfg.GnmiOptions.Encoding))]; !exists {
		return nil, fmt.Errorf("encoding %q not supported", cfg.GnmiOptions.Encoding)
	}

	return gt, nil
}

func (t *gnmiTarget) Subscribe(ctx context.Context, req *gnmi.SubscribeRequest, subscriptionName string) (chan *gnmi.SubscribeResponse, chan error) {
	return t.target.SubscribeStreamChan(ctx, req, subscriptionName)
}

func (t *gnmiTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	var err error
	gnmiReq := &gnmi.GetRequest{
		Path: make([]*gnmi.Path, 0, len(req.GetPath())),
	}
	for _, p := range req.GetPath() {
		gnmiReq.Path = append(gnmiReq.Path, utils.ToGNMIPath(p))
	}

	// convert sdcpb data type to gnmi data type
	gnmiReq.Type, err = gnmiutils.SdcpbDataTypeToGNMIType(req.GetDataType())
	if err != nil {
		return nil, err
	}

	// convert sdcpb encoding to gnmi encoding
	gnmiReq.Encoding, err = gnmiutils.SdcpbEncodingToGNMIENcoding(req.Encoding)
	if err != nil {
		return nil, err
	}
	// execute the gnmi get
	gnmiRsp, err := t.target.Get(ctx, gnmiReq)
	if err != nil {
		return nil, err
	}

	schemaRsp := &sdcpb.GetDataResponse{
		Notification: make([]*sdcpb.Notification, 0, len(gnmiRsp.GetNotification())),
	}
	for _, n := range gnmiRsp.GetNotification() {
		sn := utils.ToSchemaNotification(n)
		schemaRsp.Notification = append(schemaRsp.Notification, sn)
	}
	return schemaRsp, nil
}

func (t *gnmiTarget) Set(ctx context.Context, source targetTypes.TargetSource) (*sdcpb.SetDataResponse, error) {
	var upds []*sdcpb.Update
	var deletes []*sdcpb.Path
	var err error

	if t == nil {
		return nil, fmt.Errorf("%s", "not connected")
	}

	// deletes from protos
	deletes, err = source.ToProtoDeletes(ctx)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(t.cfg.GnmiOptions.Encoding) {
	case "json":
		jsonData, err := source.ToJson(true)
		if err != nil {
			return nil, err
		}
		if jsonData != nil {
			jsonBytes, err := json.Marshal(jsonData)
			if err != nil {
				return nil, err
			}
			upds = []*sdcpb.Update{{Path: &sdcpb.Path{}, Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: jsonBytes}}}}
		}

	case "json_ietf":
		jsonData, err := source.ToJsonIETF(true)
		if err != nil {
			return nil, err
		}
		if jsonData != nil {
			jsonBytes, err := json.Marshal(jsonData)
			if err != nil {
				return nil, err
			}
			upds = []*sdcpb.Update{{Path: &sdcpb.Path{}, Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: jsonBytes}}}}
		}

	case "proto":
		upds, err = source.ToProtoUpdates(ctx, true)
		if err != nil {
			return nil, err
		}
	}

	setReq := &gnmi.SetRequest{
		Delete: make([]*gnmi.Path, 0, len(deletes)),
		Update: make([]*gnmi.Update, 0, len(upds)),
	}
	for _, del := range deletes {
		gdel := utils.ToGNMIPath(del)
		setReq.Delete = append(setReq.Delete, gdel)
	}
	for _, upd := range upds {
		gupd := t.convertKeyUpdates(upd)
		setReq.Update = append(setReq.Update, gupd)
	}

	log.Debugf("gnmi set request:\n%s", prototext.Format(setReq))

	rsp, err := t.target.Set(ctx, setReq)
	if err != nil {
		return nil, err
	}
	schemaSetRsp := &sdcpb.SetDataResponse{
		Response:  make([]*sdcpb.UpdateResult, 0, len(rsp.GetResponse())),
		Timestamp: rsp.GetTimestamp(),
	}
	for _, updr := range rsp.GetResponse() {
		schemaSetRsp.Response = append(schemaSetRsp.Response, &sdcpb.UpdateResult{
			Path: utils.FromGNMIPath(rsp.GetPrefix(), updr.GetPath()),
			Op:   sdcpb.UpdateResult_Operation(updr.GetOp()),
		})
	}
	return schemaSetRsp, nil
}

func (t *gnmiTarget) Status() *targetTypes.TargetStatus {
	result := targetTypes.NewTargetStatus(targetTypes.TargetStatusNotConnected)

	if t == nil || t.target == nil {
		result.Details = "connection not initialized"
		return result
	}
	switch t.target.ConnState() {
	case connectivity.Ready.String(), connectivity.Idle.String():
		result.Status = targetTypes.TargetStatusConnected
		result.Details = t.target.ConnState()
	case connectivity.Connecting.String(), connectivity.Shutdown.String(), connectivity.TransientFailure.String():
		result.Status = targetTypes.TargetStatusNotConnected
		result.Details = t.target.ConnState()
	}

	return result
}

func (t *gnmiTarget) AddSyncs(ctx context.Context, sps ...*config.SyncProtocol) error {
	var g GnmiSync
	var err error
	for _, sp := range sps {
		switch sp.Mode {
		case "once":
			g = NewOnceSync(ctx, t, sp, t.runningStore, t.taskpoolFactory)
		case "get":
			g, err = NewGetSync(ctx, t, sp, t.runningStore, t.schemaClient)
			if err != nil {
				return err
			}
		default:
			g = NewStreamSync(ctx, t, sp, t.runningStore, t.schemaClient, t.taskpoolFactory)
		}
		t.syncs[sp.Name] = g

		err := g.Start()
		if err != nil {
			return err
		}
	}
	return nil
}

// func (t *gnmiTarget) Sync(octx context.Context, syncConfig *config.Sync) {
// 	if t != nil && t.target != nil && t.target.Config != nil {
// 		log.Infof("starting target %s sync", t.target.Config.Name)
// 	}
// 	var cancel context.CancelFunc
// 	var ctx context.Context
// 	var err error
// START:
// 	if cancel != nil {
// 		cancel()
// 	}
// 	ctx, cancel = context.WithCancel(octx)
// 	defer cancel()

// 	// todo: do not run read subscriptions for GET
// 	for _, gnmiSync := range syncConfig.Config {
// 		switch gnmiSync.Mode {
// 		case "once":
// 			err = t.periodicSync(ctx, gnmiSync)
// 		case "get":
// 			err = t.getSync(ctx, gnmiSync)
// 		default:
// 			err = t.streamSync(ctx, gnmiSync)
// 		}
// 		if err != nil {
// 			log.Errorf("target=%s: failed to sync: %v", t.target.Config.Name, err)
// 			time.Sleep(syncRetryWaitTime)
// 			goto START
// 		}
// 	}

// 	defer t.target.StopSubscriptions()

// 	rspch, errCh := t.target.ReadSubscriptions()
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			if !errors.Is(ctx.Err(), context.Canceled) {
// 				log.Errorf("datastore %s sync stopped: %v", t.target.Config.Name, ctx.Err())
// 			}
// 			return
// 		case rsp := <-rspch:
// 			switch r := rsp.Response.Response.(type) {
// 			case *gnmi.SubscribeResponse_Update:
// 				syncCh <- &SyncUpdate{
// 					Store:  rsp.SubscriptionName,
// 					Update: utils.ToSchemaNotification(r.Update),
// 				}
// 			}
// 		case err := <-errCh:
// 			if err.Err != nil {
// 				t.target.StopSubscriptions()
// 				log.Errorf("%s: sync subscription failed: %v", t.target.Config.Name, err)
// 				time.Sleep(time.Second)
// 				goto START
// 			}
// 		}
// 	}
// }

func (t *gnmiTarget) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if t.target == nil {
		return nil
	}
	return t.target.Close()
}

func (t *gnmiTarget) convertKeyUpdates(upd *sdcpb.Update) *gnmi.Update {
	if !pathIsKeyAsLeaf(upd.GetPath()) {
		return &gnmi.Update{
			Path: utils.ToGNMIPath(upd.GetPath()),
			Val:  utils.ToGNMITypedValue(upd.GetValue()),
		}
	}
	// convert key as leaf to jsonVal
	numPElem := len(upd.GetPath().GetElem())
	key := upd.GetPath().GetElem()[numPElem-1].GetName()
	valm := map[string]string{
		key: upd.GetValue().ToString(),
	}
	b, _ := json.Marshal(valm)
	var val *sdcpb.TypedValue
	if _, ok := t.encodings[gnmi.Encoding_JSON_IETF]; ok {
		val = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{
			JsonIetfVal: b,
		}}
	} else if _, ok := t.encodings[gnmi.Encoding_JSON]; ok {
		val = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{
			JsonVal: b,
		}}
	}

	// modify path
	p := proto.Clone(upd.GetPath()).(*sdcpb.Path)
	// p := upd.GetPath()
	p.Elem = p.GetElem()[:numPElem-1]
	// TODO: REVISIT: remove key from the last elem
	// delete(p.GetElem()[len(upd.GetPath().GetElem())-1].Key, key)
	return &gnmi.Update{
		Path: utils.ToGNMIPath(p),
		Val:  utils.ToGNMITypedValue(val),
	}
}

func pathIsKeyAsLeaf(p *sdcpb.Path) bool {
	numPElem := len(p.GetElem())
	if numPElem < 2 {
		return false
	}

	_, ok := p.GetElem()[numPElem-2].GetKey()[p.GetElem()[numPElem-1].GetName()]
	return ok
}
