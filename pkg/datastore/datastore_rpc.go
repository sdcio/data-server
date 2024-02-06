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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/config"
	"github.com/iptecharch/data-server/pkg/datastore/clients"
	"github.com/iptecharch/data-server/pkg/datastore/target"
	"github.com/iptecharch/data-server/pkg/schema"
	"github.com/iptecharch/data-server/pkg/utils"
)

type Datastore struct {
	// datastore config
	config *config.DatastoreConfig

	cacheClient cache.Client

	// SBI target of this datastore
	sbi target.Target

	// schema server client
	// schemaClient sdcpb.SchemaServerClient
	schemaClient schema.Client

	// client, bound to schema and version on the schema side and to datastore name on the cache side
	// do not use directly use getValidationClient()
	_validationClientBound *clients.ValidationClient

	// sync channel, to be passed to the SBI Sync method
	synCh chan *target.SyncUpdate

	// stop cancel func
	cfn context.CancelFunc

	// intent semaphore.
	// Used by SetIntent to guarantee that
	// only one SetIntent
	// is applied at a time.
	intentMutex *sync.Mutex
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(ctx context.Context, c *config.DatastoreConfig, scc schema.Client, cc cache.Client, opts ...grpc.DialOption) *Datastore {
	ds := &Datastore{
		config:       c,
		schemaClient: scc,
		cacheClient:  cc,
		intentMutex:  new(sync.Mutex),
	}
	if c.Sync != nil {
		ds.synCh = make(chan *target.SyncUpdate, c.Sync.Buffer)
	}
	ctx, cancel := context.WithCancel(ctx)
	ds.cfn = cancel

	// create cache instance if needed
	// this is a blocking  call
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
	}()
	return ds
}

func (d *Datastore) initCache(ctx context.Context) {
START:
	ok, err := d.cacheClient.Exists(ctx, d.config.Name)
	if err != nil {
		log.Errorf("failed to check cache instance %s, %s", d.config.Name, err)
		time.Sleep(time.Second)
		goto START
	}
	if ok {
		log.Debugf("cache %q already exists", d.config.Name)
		return
	}

	log.Infof("cache %s does not exist, creating it", d.config.Name)
CREATE:
	err = d.cacheClient.Create(ctx, d.config.Name, false, false)
	if err != nil {
		log.Errorf("failed to create cache %s: %v", d.config.Name, err)
		time.Sleep(time.Second)
		goto CREATE
	}
}

func (d *Datastore) connectSBI(ctx context.Context, opts ...grpc.DialOption) error {
	var err error
	sc := d.Schema().GetSchema()
	d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, sc, opts...)
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
			d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, sc, opts...)
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

func (d *Datastore) Candidates(ctx context.Context) ([]*sdcpb.DataStore, error) {
	cand, err := d.cacheClient.GetCandidates(ctx, d.Name())
	if err != nil {
		return nil, err
	}
	rsp := make([]*sdcpb.DataStore, 0, len(cand))
	for _, cd := range cand {
		rsp = append(rsp, &sdcpb.DataStore{
			Type:     sdcpb.Type_CANDIDATE,
			Name:     cd.CandidateName,
			Owner:    cd.Owner,
			Priority: cd.Priority,
		})
	}
	return rsp, nil
}

func (d *Datastore) Commit(ctx context.Context, req *sdcpb.CommitRequest) error {
	name := req.GetDatastore().GetName()
	if name == "" {
		return fmt.Errorf("missing candidate name")
	}
	changes, err := d.cacheClient.GetChanges(ctx, d.Config().Name, req.GetDatastore().GetName())
	if err != nil {
		return err
	}

	notification, err := d.changesToUpdates(ctx, changes)
	if err != nil {
		return err
	}
	log.Debugf("%s:%s notification:\n%s", d.Name(), name, prototext.Format(notification))
	// TODO: consider if leafref validation
	// needs to run before must statements validation

	// validate MUST statements
	for _, upd := range notification.GetUpdate() {
		log.Debugf("%s:%s validating must statement on path: %v", d.Name(), name, upd.GetPath())
		_, err = d.validateMustStatement(ctx, req.GetDatastore().GetName(), upd.GetPath(), true)
		if err != nil {
			return err
		}
	}

	for _, upd := range notification.GetUpdate() {
		log.Debugf("%s:%s validating leafRef on update: %v", d.Name(), name, upd)
		err = d.validateLeafRef(ctx, upd, name)
		if err != nil {
			return err
		}
	}
	// push updates to sbi
	sbiSet := &sdcpb.SetDataRequest{
		Update: notification.GetUpdate(),
		// Replace
		Delete: notification.GetDelete(),
	}
	log.Debugf("datastore %s/%s commit:\n%s", d.config.Name, name, prototext.Format(sbiSet))

	log.Infof("datastore %s/%s commit: sending a setDataRequest with num_updates=%d, num_replaces=%d, num_deletes=%d",
		d.config.Name, name, len(sbiSet.GetUpdate()), len(sbiSet.GetReplace()), len(sbiSet.GetDelete()))
	// send set request only if there are updates and/or deletes
	if len(sbiSet.GetUpdate())+len(sbiSet.GetReplace())+len(sbiSet.GetDelete()) > 0 {
		rsp, err := d.sbi.Set(ctx, sbiSet)
		if err != nil {
			return err
		}
		log.Debugf("datastore %s/%s SetResponse from SBI: %v", d.config.Name, name, rsp)
	}
	// commit candidate changes into the intended store
	err = d.cacheClient.Commit(ctx, d.config.Name, name)
	if err != nil {
		return err
	}

	if req.GetStay() {
		// reset candidate changes and (TODO) rebase
		return d.cacheClient.Discard(ctx, d.config.Name, name)
	}
	// delete candidate
	return d.cacheClient.DeleteCandidate(ctx, d.Name(), name)
}

func (d *Datastore) Rebase(ctx context.Context, req *sdcpb.RebaseRequest) error {
	// name := req.GetDatastore().GetName()
	// if name == "" {
	// 	return fmt.Errorf("missing candidate name")
	// }
	// d.m.Lock()
	// defer d.m.Unlock()
	// cand, ok := d.candidates[name]
	// if !ok {
	// 	return fmt.Errorf("unknown candidate name %q", name)
	// }

	// newBase, err := d.main.config.Clone()
	// if err != nil {
	// 	return fmt.Errorf("failed to rebase: %v", err)
	// }
	// cand.base = newBase
	return nil
}

func (d *Datastore) Discard(ctx context.Context, req *sdcpb.DiscardRequest) error {
	return d.cacheClient.Discard(ctx, req.GetName(), req.Datastore.GetName())
}

func (d *Datastore) CreateCandidate(ctx context.Context, ds *sdcpb.DataStore) error {
	if ds.GetPriority() < 0 {
		return fmt.Errorf("invalid priority value must be >0")
	}
	if ds.GetPriority() <= 0 {
		ds.Priority = 1
	}
	if ds.GetOwner() == "" {
		ds.Owner = DefaultOwner
	}
	return d.cacheClient.CreateCandidate(ctx, d.Name(), ds.GetName(), ds.GetOwner(), ds.GetPriority())
}

func (d *Datastore) DeleteCandidate(ctx context.Context, name string) error {
	return d.cacheClient.DeleteCandidate(ctx, d.Name(), name)
}

func (d *Datastore) ConnectionState() string {
	if d.sbi == nil {
		return ""
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

func (d *Datastore) DeleteCache(ctx context.Context) error {
	return d.cacheClient.Delete(ctx, d.config.Name)
}

func (d *Datastore) Sync(ctx context.Context) {
	// this semaphore controls the number of concurrent writes to the cache
	sem := semaphore.NewWeighted(d.config.Sync.WriteWorkers)
	go d.sbi.Sync(ctx,
		d.config.Sync,
		d.synCh,
	)

	var err error
	var pruneID string
MAIN:
	for {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				log.Errorf("datastore %s sync stopped: %v", d.Name(), ctx.Err())
			}
			return
		case syncup := <-d.synCh:
			if syncup.Start {
				log.Debugf("%s: sync start", d.Name())
				for {
					pruneID, err = d.cacheClient.CreatePruneID(ctx, d.Name(), syncup.Force)
					if err != nil {
						log.Errorf("datastore %s failed to create prune ID: %v", d.Name(), err)
						time.Sleep(time.Second)
						continue // retry
					}
					continue MAIN
				}
			}
			if syncup.End && pruneID != "" {
				log.Debugf("%s: sync end", d.Name())
				for {
					err = d.cacheClient.ApplyPrune(ctx, d.Name(), pruneID)
					if err != nil {
						log.Errorf("datastore %s failed to prune cache after update: %v", d.Name(), err)
						time.Sleep(time.Second)
						continue // retry
					}
					break
				}
				log.Debugf("%s: sync resetting pruneID", d.Name())
				pruneID = ""
				continue // MAIN FOR loop
			}
			// a regular notification
			log.Debugf("%s: sync acquire semaphore", d.Name())
			err = sem.Acquire(ctx, 1)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Infof("datastore %s sync stopped", d.config.Name)
					return
				}
				log.Errorf("failed to acquire semaphore: %v", err)
				continue
			}
			log.Debugf("%s: sync acquired semaphore", d.Name())
			go d.storeSyncMsg(ctx, syncup, sem)
		}
	}
}

func isState(r *sdcpb.GetSchemaResponse) bool {
	switch r := r.Schema.Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return r.Container.IsState
	case *sdcpb.SchemaElem_Field:
		return r.Field.IsState
	case *sdcpb.SchemaElem_Leaflist:
		return r.Leaflist.IsState
	}
	return false
}

func (d *Datastore) validateLeafRef(ctx context.Context, upd *sdcpb.Update, candidate string) error {
	done := make(chan struct{})
	ch, err := d.getValidationClient().GetSchemaElements(ctx, upd.GetPath(), done)
	if err != nil {
		return err
	}

	defer close(done)
	//
	peIndex := 0
	numPE := len(upd.GetPath().GetElem())
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sch, ok := <-ch:
			if !ok {
				return nil
			}
			if numPE < peIndex+1 {
				// should not happen if the path has been properly validated
				return fmt.Errorf("received more schema elements than pathElem")
			}
			peIndex++
			switch sch := sch.GetSchema().GetSchema().(type) {
			case *sdcpb.SchemaElem_Container:
				// check if container keys are leafrefs
				for _, keySchema := range sch.Container.GetKeys() {
					if keySchema.GetType().GetType() != "leafref" {
						continue
					}
					leafRefPath, err := utils.StripPathElemPrefix(keySchema.GetType().GetLeafref())
					if err != nil {
						return err
					}
					// get pathElem with leafRef key
					pe := upd.GetPath().GetElem()[peIndex-1]
					// get leafRef value
					leafRefValue := pe.GetKey()[keySchema.GetName()]

					lrefSdcpbPath, err := utils.ParsePath(leafRefPath)
					if err != nil {
						return err
					}
					// if it contains "./" or "../" like any relative path stuff
					// we need to resolve that
					if strings.Contains(leafRefPath, "./") {
						// make leafref path absolute
						lrefSdcpbPath, err = makeLeafRefAbs(upd.GetPath(), lrefSdcpbPath, upd.GetValue().GetStringVal())
						if err != nil {
							return err
						}
					}

					err = d.resolveLeafref(ctx, candidate, lrefSdcpbPath, leafRefValue)
					if err != nil {
						return err
					}
				}
			case *sdcpb.SchemaElem_Field:
				if sch.Field.GetType().GetType() != "leafref" {
					continue
				}
				// remove namespace elements from path /foo:interface/bar:subinterface -> /interface/subinterface
				leafRefPath, err := utils.StripPathElemPrefix(sch.Field.GetType().GetLeafref())
				if err != nil {
					return err
				}

				// convert leafref Path to sdcpb Path
				lrefSdcpbPath, err := utils.ParsePath(leafRefPath)
				if err != nil {
					return err
				}
				// if it contains "./" or "../" like any relative path stuff
				// we need to resolve that
				if strings.Contains(leafRefPath, "./") {
					// make leafref path absolute
					lrefSdcpbPath, err = makeLeafRefAbs(upd.GetPath(), lrefSdcpbPath, upd.GetValue().GetStringVal())
					if err != nil {
						return err
					}
				}

				err = d.resolveLeafref(ctx, candidate, lrefSdcpbPath, upd.GetValue().GetStringVal())
				if err != nil {
					return err
				}
			case *sdcpb.SchemaElem_Leaflist:
				if sch.Leaflist.GetType().GetType() != "leafref" {
					continue
				}
				leafRefPath, err := utils.StripPathElemPrefix(sch.Leaflist.GetType().GetLeafref())
				if err != nil {
					return err
				}
				log.Warnf("!! found leafref leaflist %s | %s", sch.Leaflist.Name, leafRefPath)
			}
		}
	}
}

func makeLeafRefAbs(base, lref *sdcpb.Path, value string) (*sdcpb.Path, error) {
	// create a result
	result := &sdcpb.Path{
		Elem: make([]*sdcpb.PathElem, 0, len(base.Elem)),
	}
	// copy base into result
	for _, x := range base.Elem {
		result.Elem = append(result.Elem, &sdcpb.PathElem{
			Name: x.GetName(),
			Key:  copyMap(x.GetKey()),
		})
	}
	// process leafref elements and adjust result
	for _, lrefElem := range lref.Elem {
		// if .. in path, remove last elem from result (move up)
		if lrefElem.GetName() == ".." {
			if len(result.Elem) == 0 {
				return nil, fmt.Errorf("invalid leafref path %s based on %s", lref.String(), base.String())
			}
			result.Elem = result.Elem[:len(result.Elem)-1]
			continue
		}
		if lrefElem.GetName() == "." {
			// no one knows if this is a valid case, but we voted and here it is :-P
			continue
		}

		// if proper path elem, add to path
		result.Elem = append(result.Elem, lrefElem)
	}

	return result, nil
}

func (d *Datastore) resolveLeafref(ctx context.Context, candidate string, leafRefPath *sdcpb.Path, value string) error {

	// Subsequent Process:
	// now we remove the last element of the referenced path
	// adding its name to the one before last element as a key
	// with the value of the item that we're validating the leafref for

	// get the schema for results paths last element
	schemaResp, err := d.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path:   &sdcpb.Path{Elem: leafRefPath.Elem[:len(leafRefPath.Elem)-1]},
		Schema: d.Schema().GetSchema(),
	})
	if err != nil {
		return err
	}
	// check for the schema defined keys
	for _, k := range schemaResp.GetSchema().GetContainer().GetKeys() {
		// if the last element of results path is a key
		if k.Name == leafRefPath.GetElem()[len(leafRefPath.Elem)-1].GetName() {
			// check if the one before last has a key map initialized
			if leafRefPath.Elem[len(leafRefPath.Elem)-2].GetKey() == nil {
				// create map otherwise
				leafRefPath.Elem[len(leafRefPath.Elem)-2].Key = map[string]string{}
			}
			// add the value as a key value under the last elements name to the one before last elemnt key list
			leafRefPath.Elem[len(leafRefPath.Elem)-2].Key[leafRefPath.Elem[len(leafRefPath.Elem)-1].Name] = value
			// remove the last elem, we now have the key value stored in the one before last
			leafRefPath.Elem = leafRefPath.Elem[:len(leafRefPath.Elem)-1]
			return nil
		}
	}

	// TODO: update when stored values are not stringVal anymore
	data, err := d.getValidationClient().GetValue(ctx, candidate, leafRefPath)
	if err != nil {
		return err
	}

	if data == nil {
		return fmt.Errorf("missing leaf reference %q: %q", leafRefPath, value)
	}
	return nil
}

func (d *Datastore) storeSyncMsg(ctx context.Context, syncup *target.SyncUpdate, sem *semaphore.Weighted) {
	defer sem.Release(1)

	cNotification, err := d.convertNotificationTypedValues(ctx, syncup.Update)
	if err != nil {
		log.Errorf("failed to convert notification typedValue: %v", err)
		return
	}

	for _, del := range cNotification.GetDelete() {
		store := cachepb.Store_CONFIG
		if d.config.Sync != nil && d.config.Sync.Validate {
			scRsp, err := d.getSchema(ctx, del)
			if err != nil {
				log.Errorf("datastore %s failed to get schema for delete path %v: %v", d.config.Name, del, err)
				continue
			}
			if isState(scRsp) {
				store = cachepb.Store_STATE
			}
		}
		delPath := utils.ToStrings(del, false, false)
		rctx, cancel := context.WithTimeout(ctx, time.Minute) // TODO:
		defer cancel()
		err = d.cacheClient.Modify(rctx, d.Config().Name,
			&cache.Opts{
				Store: store,
			},
			[][]string{delPath}, nil)
		if err != nil {
			log.Errorf("datastore %s failed to delete path %v: %v", d.config.Name, delPath, err)
		}
	}

	for _, upd := range cNotification.GetUpdate() {
		store := cachepb.Store_CONFIG
		if d.config.Sync != nil && d.config.Sync.Validate {
			scRsp, err := d.getSchema(ctx, upd.GetPath())
			if err != nil {
				log.Errorf("datastore %s failed to get schema for update path %v: %v", d.config.Name, upd.GetPath(), err)
				continue
			}
			if isState(scRsp) {
				store = cachepb.Store_STATE
			}
		}

		// TODO:[KR] convert update typedValue if needed
		cUpd, err := d.cacheClient.NewUpdate(upd)
		if err != nil {
			log.Errorf("datastore %s failed to create update from %v: %v", d.config.Name, upd, err)
			continue
		}

		rctx, cancel := context.WithTimeout(ctx, time.Minute) // TODO:[KR] make this timeout configurable ?
		defer cancel()
		err = d.cacheClient.Modify(rctx, d.Config().Name, &cache.Opts{
			Store: store,
		}, nil, []*cache.Update{cUpd})
		if err != nil {
			log.Errorf("datastore %s failed to send modify request to cache: %v", d.config.Name, err)
		}
	}
}

// helper for GetSchema
func (d *Datastore) getSchema(ctx context.Context, p *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
	return d.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path:   p,
		Schema: d.Schema().GetSchema(),
	})
}

func (d *Datastore) validatePath(ctx context.Context, p *sdcpb.Path) error {
	_, err := d.getSchema(ctx, p)
	return err
}

func (d *Datastore) toPath(ctx context.Context, p []string) (*sdcpb.Path, error) {
	rsp, err := d.schemaClient.ToPath(ctx, &sdcpb.ToPathRequest{
		PathElement: p,
		Schema: &sdcpb.Schema{
			Name:    d.Schema().Name,
			Vendor:  d.Schema().Vendor,
			Version: d.Schema().Version,
		},
	})
	if err != nil {
		return nil, err
	}
	return rsp.GetPath(), nil
}

func (d *Datastore) changesToUpdates(ctx context.Context, changes []*cache.Change) (*sdcpb.Notification, error) {
	notif := &sdcpb.Notification{
		Update: make([]*sdcpb.Update, 0, len(changes)),
		Delete: make([]*sdcpb.Path, 0, len(changes)),
	}
	for _, change := range changes {
		if change == nil {
			continue
		}
		switch {
		case len(change.Delete) != 0:
			p, err := d.toPath(ctx, change.Delete)
			if err != nil {
				return nil, err
			}
			notif.Delete = append(notif.Delete, p)
		case change.Update != nil:
			tv, err := change.Update.Value()
			if err != nil {
				return nil, err
			}
			p, err := d.toPath(ctx, change.Update.GetPath())
			if err != nil {
				return nil, err
			}
			upd := &sdcpb.Update{
				Path:  p,
				Value: tv,
			}
			notif.Update = append(notif.Update, upd)
		}
	}
	return notif, nil
}

// getValidationClient will create a ValidationClient instance if not already existing
// save it as part of the datastore and return a valid *clients.ValidationClient
func (d *Datastore) getValidationClient() *clients.ValidationClient {
	// if not initialized, init it, cache it
	if d._validationClientBound == nil {
		d._validationClientBound = clients.NewValidationClient(d.Name(), d.cacheClient, d.Schema().GetSchema(), d.schemaClient)
	}
	// return the bound validation client
	return d._validationClientBound
}

// conversion
type leafListNotification struct {
	path      *sdcpb.Path
	leaflists []*sdcpb.TypedValue
}

func (d *Datastore) convertNotificationTypedValues(ctx context.Context, n *sdcpb.Notification) (*sdcpb.Notification, error) {
	// this map serves as a context to group leaf-lists
	// sent as keys in separate updates.
	leaflists := map[string]*leafListNotification{}
	nn := &sdcpb.Notification{
		Timestamp: n.GetTimestamp(),
		Update:    make([]*sdcpb.Update, 0, len(n.GetUpdate())),
		Delete:    n.GetDelete(),
	}
	// convert typed values to their YANG type
	for _, upd := range n.GetUpdate() {
		scRsp, err := d.getSchema(ctx, upd.GetPath())
		if err != nil {
			return nil, err
		}
		nup, err := d.convertUpdateTypedValue(ctx, upd, scRsp, leaflists)
		if err != nil {
			return nil, err
		}
		log.Debugf("%s: converted update from: %v, to: %v", d.Name(), upd, nup)
		if nup == nil { // filters out notification ending in non-presence containers
			continue
		}
		nn.Update = append(nn.Update, nup)
	}
	// add accumulated leaf-lists
	for _, lfnotif := range leaflists {
		nn.Update = append(nn.Update, &sdcpb.Update{
			Path: lfnotif.path,
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{Element: lfnotif.leaflists},
			},
			},
		})
	}

	return nn, nil
}

func (d *Datastore) convertUpdateTypedValue(ctx context.Context, upd *sdcpb.Update, scRsp *sdcpb.GetSchemaResponse, leaflists map[string]*leafListNotification) (*sdcpb.Update, error) {
	switch {
	case scRsp.GetSchema().GetContainer() != nil:
		if !scRsp.GetSchema().GetContainer().GetIsPresence() {
			return nil, nil
		}
		return upd, nil
	case scRsp.GetSchema().GetLeaflist() != nil:
		// leaf-list received as a key
		if upd.GetValue() == nil {
			// clone path
			p := proto.Clone(upd.GetPath()).(*sdcpb.Path)
			// grab the key from the last elem, that's the leaf-list value
			var lftv *sdcpb.TypedValue
			for _, v := range p.GetElem()[len(p.GetElem())-1].GetKey() {
				lftv = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
				break
			}
			if lftv == nil {
				return nil, fmt.Errorf("malformed leaf-list update: %v", upd)
			}
			// delete the key from the last elem (that's the leaf-list value)
			p.GetElem()[len(p.GetElem())-1].Key = nil
			// build unique path
			sp := utils.ToXPath(p, false)
			if _, ok := leaflists[sp]; !ok {
				leaflists[sp] = &leafListNotification{
					path:      p,                               // modified path
					leaflists: make([]*sdcpb.TypedValue, 0, 1), // at least one elem
				}
			}
			// convert leaf-list to its YANG type
			clftv, err := d.typedValueToYANGType(lftv, scRsp.GetSchema())
			if err != nil {
				return nil, err
			}
			// append leaf-list
			leaflists[sp].leaflists = append(leaflists[sp].leaflists, clftv)
			return nil, nil
		}
		// regular leaf list
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			return upd, nil
		default:
			return nil, fmt.Errorf("unexpected leaf-list typedValue: %v", upd.GetValue())
		}
	case scRsp.GetSchema().GetField() != nil:
		ctv, err := d.typedValueToYANGType(upd.GetValue(), scRsp.GetSchema())
		if err != nil {
			return nil, err
		}
		return &sdcpb.Update{
			Path:  upd.GetPath(),
			Value: ctv,
		}, nil
	}
	return nil, nil
}

func (d *Datastore) typedValueToYANGType(tv *sdcpb.TypedValue, schemaObject *sdcpb.SchemaElem) (*sdcpb.TypedValue, error) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_AsciiVal:
		return convertToTypedValue(schemaObject, tv.GetAsciiVal(), tv.GetTimestamp())
	case *sdcpb.TypedValue_BoolVal:
		return tv, nil
	case *sdcpb.TypedValue_BytesVal:
		return tv, nil
	case *sdcpb.TypedValue_DecimalVal:
		return tv, nil
	case *sdcpb.TypedValue_FloatVal:
		return tv, nil
	case *sdcpb.TypedValue_DoubleVal:
		return tv, nil
	case *sdcpb.TypedValue_IntVal:
		return tv, nil
	case *sdcpb.TypedValue_StringVal:
		return convertToTypedValue(schemaObject, tv.GetStringVal(), tv.GetTimestamp())
	case *sdcpb.TypedValue_UintVal:
		return tv, nil
	case *sdcpb.TypedValue_JsonIetfVal: // TODO:
	case *sdcpb.TypedValue_JsonVal: // TODO:
	case *sdcpb.TypedValue_LeaflistVal:
		return tv, nil
	case *sdcpb.TypedValue_ProtoBytes:
		return tv, nil
	case *sdcpb.TypedValue_AnyVal:
		return tv, nil
	}
	return tv, nil
}

func convertToTypedValue(schemaObject *sdcpb.SchemaElem, v string, ts uint64) (*sdcpb.TypedValue, error) {
	var schemaType *sdcpb.SchemaLeafType
	switch {
	case schemaObject.GetField() != nil:
		schemaType = schemaObject.GetField().GetType()
	case schemaObject.GetLeaflist() != nil:
		schemaType = schemaObject.GetLeaflist().GetType()
	case schemaObject.GetContainer() != nil:
		if !schemaObject.GetContainer().IsPresence {
			return nil, errors.New("non presence container update")
		}
		return nil, nil
	}
	return convertStringToTv(schemaType, v, ts)
}

func convertStringToTv(schemaType *sdcpb.SchemaLeafType, v string, ts uint64) (*sdcpb.TypedValue, error) {
	// convert field or leaf-list schema elem
	switch schemaType.GetType() {
	case "string":
		return &sdcpb.TypedValue{

			Value: &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "uint64", "uint32", "uint16", "uint8":
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_UintVal{UintVal: uint64(i)},
		}, nil
	case "int64", "int32", "int16", "int8":
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_IntVal{IntVal: int64(i)},
		}, nil
	case "boolean":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_BoolVal{BoolVal: b},
		}, nil
	case "decimal64":
		// TODO: convert string to decimal
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DecimalVal{DecimalVal: &sdcpb.Decimal64{}},
		}, nil
	case "identityref":
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "leafref": // TODO: query leafref type
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "union":
		for _, ut := range schemaType.GetUnionTypes() {
			tv, err := convertStringToTv(ut, v, ts)
			if err == nil {
				return tv, nil
			}
		}
		return nil, fmt.Errorf("invalid value %s for union type: %v", v, schemaType)
	case "enumeration":
		// TODO: get correct type, assuming string
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "": // presence ?
		return &sdcpb.TypedValue{}, nil
	}
	return nil, nil
}
