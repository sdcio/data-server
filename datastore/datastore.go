package datastore

import (
	"context"
	"fmt"
	"time"

	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/data-server/cache"
	"github.com/iptecharch/data-server/config"
	"github.com/iptecharch/data-server/datastore/clients"
	"github.com/iptecharch/data-server/datastore/target"
	"github.com/iptecharch/data-server/schema"
	"github.com/iptecharch/data-server/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
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
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(c *config.DatastoreConfig, scc schema.Client, cc cache.Client, opts ...grpc.DialOption) *Datastore {
	ds := &Datastore{
		config:       c,
		schemaClient: scc,
		cacheClient:  cc,
	}
	if c.Sync != nil {
		ds.synCh = make(chan *target.SyncUpdate, c.Sync.Buffer)
	}
	ctx, cancel := context.WithCancel(context.TODO())
	ds.cfn = cancel

	// create cache instance if needed
	// this is a blocking  call
	ds.initCache(ctx)

	// init sbi, this is a blocking call
	ds.connectSBI(ctx, opts...)

	// start syncing goroutine
	if c.Sync != nil {
		go ds.Sync(ctx)
	}
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

	log.Infof("cache %s does not exist creating it", d.config.Name)
CREATE:
	err = d.cacheClient.Create(ctx, d.config.Name, false, false)
	if err != nil {
		log.Errorf("failed to create cache %s: %v", d.config.Name, err)
		time.Sleep(time.Second)
		goto CREATE
	}
}

func (d *Datastore) connectSBI(ctx context.Context, opts ...grpc.DialOption) {
	var err error
	sc := d.Schema().GetSchema()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sbi, err = target.New(ctx, d.config.Name, d.config.SBI, d.schemaClient, sc, opts...)
			if err != nil {
				log.Errorf("failed to create DS %s target: %v", d.config.Name, err)
				continue
			}
			return
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

func (d *Datastore) Candidates(ctx context.Context) ([]string, error) {
	return d.cacheClient.GetCandidates(ctx, d.Name())
}

func (d *Datastore) Commit(ctx context.Context, req *sdcpb.CommitRequest) error {
	name := req.GetDatastore().GetName()
	if name == "" {
		return fmt.Errorf("missing candidate name")
	}
	if req.GetRebase() {
		fmt.Println("TODO: implement candidate base in cache")
	}
	changes, err := d.cacheClient.GetChanges(ctx, d.Config().Name, req.GetDatastore().GetName())
	if err != nil {
		return err
	}
	log.Debugf("%s:%s changes: %v", d.Name(), name, changes)
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
		_, err = d.validateMustStatement(ctx, req.GetDatastore().GetName(), upd.GetPath())
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
	rsp, err := d.sbi.Set(ctx, sbiSet)
	if err != nil {
		return err
	}
	log.Debugf("datastore %s/%s SetResponse from SBI: %v", d.config.Name, name, rsp)
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

func (d *Datastore) CreateCandidate(ctx context.Context, name string) error {
	return d.cacheClient.CreateCandidate(ctx, d.Name(), name)
}

func (d *Datastore) DeleteCandidate(ctx context.Context, name string) error {
	return d.cacheClient.DeleteCandidate(ctx, d.Name(), name)
}

func (d *Datastore) Stop() {
	d.cfn()
	d.cacheClient.Close()
}

func (d *Datastore) Sync(ctx context.Context) {
	// this semaphore controls the number of concurrent writes to the cache
	sem := semaphore.NewWeighted(d.config.Sync.WriteWorkers)
	go d.sbi.Sync(ctx, d.config.Sync, d.synCh)
	var err error
	for {
		select {
		case <-ctx.Done():
			log.Errorf("datastore %s sync stopped: %v", d.config.Name, ctx.Err())
			return
		case syncup := <-d.synCh:
			err = sem.Acquire(ctx, 1)
			if err != nil {
				log.Errorf("failed to acquire semaphore: %v", err)
				continue
			}
			// go
			go d.storeSyncMsg(ctx, syncup, sem)
		}
	}
}

// func isState(r *sdcpb.GetSchemaResponse) bool {
// 	switch r := r.Schema.(type) {
// 	case *sdcpb.GetSchemaResponse_Container:
// 		return r.Container.IsState
// 	case *sdcpb.GetSchemaResponse_Field:
// 		return r.Field.IsState
// 	case *sdcpb.GetSchemaResponse_Leaflist:
// 		return r.Leaflist.IsState
// 	}
// 	return false
// }

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
					//
					err = d.resolveLeafref(ctx, candidate, leafRefPath, leafRefValue)
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

				err = d.resolveLeafref(ctx, candidate, leafRefPath, upd.GetValue().GetStringVal())
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

func (d *Datastore) resolveLeafref(ctx context.Context, candidate, leafRefPath string, value string) error {
	// parse the string into a *sdcpb.Path
	p, err := utils.ParsePath(leafRefPath)
	if err != nil {
		return err
	}

	// Subsequent Process:
	// now we remove the last element of the referenced path
	// adding its name to the one before last element as a key
	// with the value of the item that we're validating the leafref for

	pLen := len(p.GetElem())
	// extract one before the last path elements element
	keyAddedLastElem := p.GetElem()[pLen-2]
	// add the Key map
	if keyAddedLastElem.GetKey() == nil {
		keyAddedLastElem.Key = map[string]string{}
	}
	// add the updates value as the value under the key, which is the initial last element of the leafref path
	keyAddedLastElem.Key[p.GetElem()[pLen-1].GetName()] = value
	// reconstruct the path, the leafref points to, with the path except for the last two elements
	// and the altered penultimate element
	p.Elem = append(p.GetElem()[:pLen-2], keyAddedLastElem)

	// TODO: update when stored values are not stringVal anymore
	data, err := d.getValidationClient().GetValue(ctx, candidate, p)
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
	dels := make([][]string, 0, len(syncup.Update.GetDelete()))
	upds := make([]cache.Update, 0, len(syncup.Update.GetUpdate()))

	store := cachepb.Store_CONFIG

	if syncup.Tree == "state" {
		store = cachepb.Store_STATE
	}
	for _, del := range syncup.Update.GetDelete() {
		if d.config.Sync != nil && d.config.Sync.Validate {
			scRsp, err := d.getSchema(ctx, del)
			if err != nil {
				log.Errorf("datastore %s failed to get schema for delete path %v: %v", d.config.Name, del, err)
				continue
			}
			_ = scRsp
		}
		dels = append(dels, utils.ToStrings(del, false, false))
	}

	for _, upd := range syncup.Update.GetUpdate() {
		if d.config.Sync != nil && d.config.Sync.Validate {
			scRsp, err := d.getSchema(ctx, upd.GetPath())
			if err != nil {
				log.Errorf("datastore %s failed to get schema for update path %v: %v", d.config.Name, upd.GetPath(), err)
				continue
			}
			// workaround, skip presence containers
			switch r := scRsp.GetSchema().Schema.(type) {
			case *sdcpb.SchemaElem_Container:
				if r.Container.IsPresence {
					continue
				}
			}
			// _ = scRsp // TODO validate value
		}

		cUpd, err := d.cacheClient.NewUpdate(upd)
		if err != nil {
			log.Errorf("failed to create update from %v: %v", upd, err)
			continue
		}
		upds = append(upds, cUpd)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute) // TODO:
	defer cancel()
	err := d.cacheClient.Modify(ctx, d.Config().Name, store, dels, upds)
	if err != nil {
		log.Errorf("failed to send modify request to cache: %v", err)
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
		switch {
		case len(change.Delete) != 0:
			p, err := d.toPath(ctx, change.Delete)
			if err != nil {
				return nil, err
			}
			notif.Delete = append(notif.Delete, p)
		default:
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
