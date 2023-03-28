package datastore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	"github.com/iptecharch/schema-server/datastore/target"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type Datastore struct {
	// datastore config
	config *config.DatastoreConfig
	// main config+state trees
	main *main

	// map of candidates
	m          *sync.RWMutex
	candidates map[string]*candidate

	// SBI target of this datastore
	sbi target.Target

	// schema server client
	schemaClient schemapb.SchemaServerClient

	// sync channel, to be passed to the SBI Sync method
	synCh chan *target.SyncUpdate

	// stop cancel func
	cfn context.CancelFunc
}

type main struct {
	config *ctree.Tree
	state  *ctree.Tree
}

// candidate is a "fork" of Datastore main config tree,
// it holds the list of changes (deletes, replaces, updates) sent towards it,
// a clone of the main config tree when the candidate was created as well as a
// "head" tree.
type candidate struct {
	base *ctree.Tree
	head *ctree.Tree

	m        *sync.RWMutex
	updates  []*schemapb.Update
	replaces []*schemapb.Update
	deletes  []*schemapb.Path
}

// New creates a new datastore, its schema server client and initializes the SBI target
// func New(c *config.DatastoreConfig, schemaServer *config.RemoteSchemaServer) *Datastore {
func New(c *config.DatastoreConfig, scc schemapb.SchemaServerClient) *Datastore {
	ds := &Datastore{
		config:       c,
		main:         &main{config: &ctree.Tree{}, state: &ctree.Tree{}},
		m:            &sync.RWMutex{},
		candidates:   map[string]*candidate{},
		schemaClient: scc,
		synCh:        make(chan *target.SyncUpdate),
	}
	ctx, cancel := context.WithCancel(context.TODO())
	ds.cfn = cancel

	ds.connectSBI(ctx, c)

	go ds.Sync(ctx)
	return ds
}

func (d *Datastore) connectSBI(ctx context.Context, c *config.DatastoreConfig) {
	var err error
	sc := &schemapb.Schema{
		Name:    d.Schema().Name,
		Vendor:  d.Schema().Vendor,
		Version: d.Schema().Version,
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
OUT:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sbi, err = target.New(ctx, c.Name, c.SBI, d.schemaClient, sc)
			if err != nil {
				log.Errorf("failed to create DS %s target: %v", c.Name, err)
				continue
			}
			break OUT
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

func (d *Datastore) Candidates() []string {
	d.m.RLock()
	defer d.m.RUnlock()
	rs := make([]string, 0)
	for c := range d.candidates {
		rs = append(rs, c)
	}
	return rs
}

func (d *Datastore) Commit(ctx context.Context, req *schemapb.CommitRequest) error {
	name := req.GetDatastore().GetName()
	if name == "" {
		return fmt.Errorf("missing candidate name")
	}
	d.m.Lock()
	defer d.m.Unlock()
	cand, ok := d.candidates[name]
	if !ok {
		return fmt.Errorf("unknown candidate name %q", name)
	}
	if req.GetRebase() {
		newBase, err := d.main.config.Clone()
		if err != nil {
			return fmt.Errorf("failed to rebase: %v", err)
		}
		cand.base = newBase
	}
	resTree, err := cand.base.Clone()
	if err != nil {
		return err
	}
	for _, repl := range cand.replaces {
		err = resTree.AddSchemaUpdate(repl)
		if err != nil {
			return err
		}
	}
	for _, upd := range cand.updates {
		err = resTree.AddSchemaUpdate(upd)
		if err != nil {
			return err
		}
	}
	// fmt.Println(resTree.PrintTree())
	// resTree.Print("")
	// TODO: 1. validate resTree
	// TODO: 2. validate added/removed leafrefs ?

	// push updates to sbi
	sbiSet := &schemapb.SetDataRequest{
		Update:  cand.updates,
		Replace: cand.replaces,
		Delete:  cand.deletes,
	}
	log.Debugf("datastore %s/%s commit: %v", d.config.Name, name, sbiSet)
	log.Infof("datastore %s/%s commit: sending a setDataRequest with num_updates=%d, num_replaces=%d, num_deletes=%d",
		d.config.Name, name, len(sbiSet.GetUpdate()), len(sbiSet.GetReplace()), len(sbiSet.GetDelete()))
	rsp, err := d.sbi.Set(ctx, sbiSet)
	if err != nil {
		return err
	}
	log.Infof("datastore %s/%s SetResponse from SBI: %v", d.config.Name, name, rsp)
	if req.GetStay() {
		// reset candidate changes and rebase
		cand.updates = make([]*schemapb.Update, 0)
		cand.replaces = make([]*schemapb.Update, 0)
		cand.deletes = make([]*schemapb.Path, 0)
		cand.base, err = d.main.config.Clone()
		return err
	}
	delete(d.candidates, name)
	return nil
}

func (d *Datastore) Rebase(ctx context.Context, req *schemapb.RebaseRequest) error {
	name := req.GetDatastore().GetName()
	if name == "" {
		return fmt.Errorf("missing candidate name")
	}
	d.m.Lock()
	defer d.m.Unlock()
	cand, ok := d.candidates[name]
	if !ok {
		return fmt.Errorf("unknown candidate name %q", name)
	}

	newBase, err := d.main.config.Clone()
	if err != nil {
		return fmt.Errorf("failed to rebase: %v", err)
	}
	cand.base = newBase
	return nil
}

func (d *Datastore) Discard(ctx context.Context, req *schemapb.DiscardRequest) error {
	d.m.Lock()
	defer d.m.Unlock()
	cand, ok := d.candidates[req.GetDatastore().GetName()]
	if !ok {
		return fmt.Errorf("unknown candidate %s", req.GetDatastore().GetName())
	}
	cand.m.Lock()
	defer cand.m.Unlock()
	cand.updates = make([]*schemapb.Update, 0)
	cand.replaces = make([]*schemapb.Update, 0)
	cand.deletes = make([]*schemapb.Path, 0)
	return nil
}

func (d *Datastore) CreateCandidate(name string) error {
	d.m.Lock()
	defer d.m.Unlock()
	base, err := d.main.config.Clone()
	if err != nil {
		return err
	}
	d.candidates[name] = &candidate{
		m:        new(sync.RWMutex),
		base:     base,
		updates:  []*schemapb.Update{},
		replaces: []*schemapb.Update{},
		deletes:  []*schemapb.Path{},
		head:     &ctree.Tree{},
	}
	return nil
}

func (d *Datastore) DeleteCandidate(name string) error {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.candidates, name)
	return nil
}

func (d *Datastore) Stop() {
	d.cfn()
}

func (d *Datastore) Sync(ctx context.Context) {
	go d.sbi.Sync(ctx, d.config.Sync, d.synCh)
	for {
		select {
		case <-ctx.Done():
			log.Errorf("datastore %s sync stopped: %v", d.config.Name, ctx.Err())
			return
		case syncup := <-d.synCh:
			for _, del := range syncup.Update.GetDelete() {
				if d.config.Sync != nil && d.config.Sync.Validate {
					scRsp, err := d.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
						Path: del,
						Schema: &schemapb.Schema{
							Name:    d.config.Schema.Name,
							Vendor:  d.config.Schema.Vendor,
							Version: d.config.Schema.Version,
						},
					})
					if err != nil {
						log.Errorf("datastore %s failed to get schema for delete path %v: %v", d.config.Name, del, err)
						continue
					}
					_ = scRsp
				}
				switch syncup.Tree {
				case "state":
					err := d.main.state.DeletePath(del)
					if err != nil {
						log.Errorf("failed to delete schema path from main state DS: %v", err)
						// log.Errorf("failed to delete schema path from main state DS: %v", n)
						continue
					}
				default:
					err := d.main.config.DeletePath(del)
					if err != nil {
						log.Errorf("failed to delete schema path from main config DS: %v", err)
						// log.Errorf("failed to delete schema path from main config DS: %v", n)
						continue
					}
				}
			}

			for _, upd := range syncup.Update.GetUpdate() {
				if d.config.Sync != nil && d.config.Sync.Validate {
					scRsp, err := d.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
						Path: upd.GetPath(),
						Schema: &schemapb.Schema{
							Name:    d.config.Schema.Name,
							Vendor:  d.config.Schema.Vendor,
							Version: d.config.Schema.Version,
						},
					})
					if err != nil {
						log.Errorf("datastore %s failed to get schema for update path %v: %v", d.config.Name, upd.GetPath(), err)
						continue
					}
					_ = scRsp // TODO validate value
				}
				switch syncup.Tree {
				case "state":
					err := d.main.state.AddSchemaUpdate(upd)
					if err != nil {
						log.Errorf("failed to insert schema update into main state DS: %v", err)
						// log.Errorf("failed to insert schema update into main state DS: %v", n)
						continue
					}
				default:
					err := d.main.config.AddSchemaUpdate(upd)
					if err != nil {
						log.Errorf("failed to insert schema update into main config DS: %v", err)
						// log.Errorf("failed to insert schema update into main config DS: %v", n)
						continue
					}
				}
			}
		}
	}
}

func isState(r *schemapb.GetSchemaResponse) bool {
	switch r := r.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		return r.Container.IsState
	case *schemapb.GetSchemaResponse_Field:
		return r.Field.IsState
	case *schemapb.GetSchemaResponse_Leaflist:
		return r.Leaflist.IsState
	}
	return false
}
