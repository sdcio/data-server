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
	"fmt"
	"sync"
	"time"

	"github.com/sdcio/cache/proto/cachepb"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils"
)

const (
	// to be used for candidates created without an owner
	DefaultOwner = "__sdcio"
)

func (d *Datastore) Get(ctx context.Context, req *sdcpb.GetDataRequest, nCh chan *sdcpb.GetDataResponse) error {
	defer close(nCh)
	switch req.GetDatastore().GetType() {
	case sdcpb.Type_MAIN:
	case sdcpb.Type_CANDIDATE:
	case sdcpb.Type_INTENDED:
		if req.GetDataType() == sdcpb.DataType_STATE {
			return status.Error(codes.InvalidArgument, "cannot query STATE data from INTENDED store")
		}
	}

	switch req.GetEncoding() {
	case sdcpb.Encoding_JSON:
	case sdcpb.Encoding_JSON_IETF:
	case sdcpb.Encoding_PROTO:
	default:
		return fmt.Errorf("unknown encoding: %v", req.GetEncoding())
	}

	var err error
	// validate that path(s) exist in the schema
	for _, p := range req.GetPath() {
		err = d.validatePath(ctx, p)
		if err != nil {
			return err
		}
	}

	// build target cache name
	name := req.GetName()
	if req.GetDatastore().GetName() != "" {
		name = fmt.Sprintf("%s/%s", req.GetName(), req.GetDatastore().GetName())
	}

	// convert sdcpb paths to a string list
	paths := make([][]string, 0, len(req.GetPath()))
	for _, p := range req.GetPath() {
		paths = append(paths, utils.ToStrings(p, false, false))
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	treeSCC := tree.NewTreeCacheClient(d.Name(), d.cacheClient)
	tc := tree.NewTreeContext(treeSCC, d.schemaClient, "")
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return err
	}

	flagsExisting := tree.NewUpdateInsertFlags()

	for _, store := range getStores(req) {
		in := d.cacheClient.ReadCh(ctx, name, &cache.Opts{
			Store:    store,
			Owner:    req.GetDatastore().GetOwner(),
			Priority: req.GetDatastore().GetPriority(),
		}, paths, 0)
	OUTER:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case upd, ok := <-in:
				if !ok {
					break OUTER
				}

				if len(upd.GetPath()) == 0 {
					continue
				}

				scp, err := d.schemaClient.ToPath(ctx, upd.GetPath())
				if err != nil {
					return err
				}
				switch len(scp.GetElem()) {
				case 0:
					continue
				case 1:
					if scp.GetElem()[0].GetName() == "" {
						continue
					}
				}
				root.AddCacheUpdateRecursive(ctx, upd, flagsExisting)
			}
		}
	}

	root.FinishInsertionPhase(ctx)

	ietf := false
	switch req.Encoding {
	case sdcpb.Encoding_JSON_IETF:
		ietf = true
		fallthrough
	case sdcpb.Encoding_JSON:
		return d.getJson(ctx, root, nCh, ietf)
	case sdcpb.Encoding_PROTO:
		return d.getProto(ctx, root, nCh)
	default:
		return fmt.Errorf("unknown encoding: %v", req.GetEncoding())
	}
}

func (d *Datastore) getProto(ctx context.Context, root *tree.RootEntry, out chan *sdcpb.GetDataResponse) error {
	upds, err := root.ToProtoUpdates(ctx, false)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		now := time.Now().UnixNano()
		out <- &sdcpb.GetDataResponse{
			Notification: []*sdcpb.Notification{
				{
					Timestamp: now,
					Update:    upds,
				},
			},
		}
		return nil
	}
}
func (d *Datastore) getJson(ctx context.Context, root *tree.RootEntry, out chan *sdcpb.GetDataResponse, ietf bool) error {

	var err error
	var j any
	// marshal map into JSON bytes
	if ietf {
		j, err = root.ToJsonIETF(false)
		if err != nil {
			return err
		}
	} else {
		j, err = root.ToJson(false)
		if err != nil {
			return err
		}
	}
	b, err := json.Marshal(j)
	if err != nil {
		err = fmt.Errorf("failed json builder indent : %v", err)
		log.Error(err)
		return err
	}
	now := time.Now().UnixNano()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- &sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{
			{
				Timestamp: now,
				Update: []*sdcpb.Update{{
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: b}},
				}},
			},
		},
	}:
	}
	return nil
}

func (d *Datastore) Subscribe(req *sdcpb.SubscribeRequest, stream sdcpb.DataServer_SubscribeServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	var err error
	for _, subsc := range req.GetSubscription() {
		err := d.doSubscribeOnce(ctx, subsc, stream)
		if err != nil {
			return err
		}
	}
	err = stream.Send(&sdcpb.SubscribeResponse{
		Response: &sdcpb.SubscribeResponse_SyncResponse{
			SyncResponse: true,
		},
	})
	if err != nil {
		return err
	}
	// start periodic gets, TODO: optimize using cache RPC
	wg := new(sync.WaitGroup)
	wg.Add(len(req.GetSubscription()))
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	for _, subsc := range req.GetSubscription() {
		go func(subsc *sdcpb.Subscription) {
			ticker := time.NewTicker(time.Duration(subsc.GetSampleInterval()))
			defer ticker.Stop()
			defer wg.Done()
			for {
				select {
				case <-doneCh:
					return
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case <-ticker.C:
					err := d.doSubscribeOnce(ctx, subsc, stream)
					if err != nil {
						errCh <- err
						close(doneCh)
						return
					}
				}
			}
		}(subsc)
	}
	wg.Wait()
	return nil
}

func (d *Datastore) doSubscribeOnce(ctx context.Context, subscription *sdcpb.Subscription, stream sdcpb.DataServer_SubscribeServer) error {
	paths := make([][]string, 0, len(subscription.GetPath()))
	for _, path := range subscription.GetPath() {
		paths = append(paths, utils.ToStrings(path, false, false))
	}

	for _, store := range getStores(subscription) {
		for upd := range d.cacheClient.ReadCh(ctx, d.config.Name, &cache.Opts{
			Store: store,
		}, paths, 0) {
			log.Debugf("ds=%s read path=%v from store=%v: %v", d.config.Name, paths, store, upd)
			rsp, err := d.subscribeResponseFromCacheUpdate(ctx, upd)
			if err != nil {
				return err
			}
			log.Debugf("ds=%s sending subscribe response: %v", d.config.Name, rsp)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				err = stream.Send(rsp)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func getStores(req proto.Message) []cachepb.Store {
	var dt sdcpb.DataType
	var candName string
	switch req := req.(type) {
	case *sdcpb.GetDataRequest:
		if req.GetDatastore().GetType() == sdcpb.Type_INTENDED {
			return []cachepb.Store{cachepb.Store_INTENDED}
		}
		dt = req.GetDataType()
		candName = req.GetDatastore().GetName()
	case *sdcpb.Subscription:
		dt = req.GetDataType()
	}

	var stores []cachepb.Store
	switch dt {
	case sdcpb.DataType_ALL:
		stores = []cachepb.Store{cachepb.Store_CONFIG}
		if candName == "" {
			stores = append(stores, cachepb.Store_STATE)
		}
	case sdcpb.DataType_CONFIG:
		stores = []cachepb.Store{cachepb.Store_CONFIG}
	case sdcpb.DataType_STATE:
		if candName == "" {
			stores = []cachepb.Store{cachepb.Store_STATE}
		}
	}
	return stores
}

func (d *Datastore) subscribeResponseFromCacheUpdate(ctx context.Context, upd *cache.Update) (*sdcpb.SubscribeResponse, error) {
	scp, err := d.schemaClient.ToPath(ctx, upd.GetPath())
	if err != nil {
		return nil, err
	}
	tv, err := upd.Value()
	if err != nil {
		return nil, err
	}
	notification := &sdcpb.Notification{
		Timestamp: time.Now().UnixNano(),
		Update: []*sdcpb.Update{{
			Path:  scp,
			Value: tv,
		}},
	}
	return &sdcpb.SubscribeResponse{
		Response: &sdcpb.SubscribeResponse_Update{
			Update: notification,
		},
	}, nil
}
