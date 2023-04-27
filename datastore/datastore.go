package datastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	"github.com/iptecharch/schema-server/datastore/target"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
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
func New(c *config.DatastoreConfig, scc schemapb.SchemaServerClient, opts ...grpc.DialOption) *Datastore {
	ds := &Datastore{
		config:       c,
		main:         &main{config: &ctree.Tree{}, state: &ctree.Tree{}},
		m:            &sync.RWMutex{},
		candidates:   map[string]*candidate{},
		schemaClient: scc,
	}
	if c.Sync != nil {
		ds.synCh = make(chan *target.SyncUpdate, c.Sync.Buffer)
	}
	ctx, cancel := context.WithCancel(context.TODO())
	ds.cfn = cancel

	ds.connectSBI(ctx, c, opts...)

	go ds.Sync(ctx)
	return ds
}

func (d *Datastore) connectSBI(ctx context.Context, c *config.DatastoreConfig, opts ...grpc.DialOption) {
	var err error
	sc := &schemapb.Schema{
		Name:    d.Schema().Name,
		Vendor:  d.Schema().Vendor,
		Version: d.Schema().Version,
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C: // kicks in after the timer
			d.sbi, err = target.New(ctx, c.Name, c.SBI, d.schemaClient, sc, opts...)
			if err != nil {
				log.Errorf("failed to create DS %s target: %v", c.Name, err)
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

	// close base and apply changes
	resTree, err := cand.base.Clone()
	if err != nil {
		return err
	}
	for _, del := range cand.deletes {
		err = resTree.DeletePath(del)
		if err != nil {
			return err
		}
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

	// validate leaf references
	for _, repl := range cand.replaces {
		err = d.validateLeafRef(ctx, resTree, repl)
		if err != nil {
			return err
		}
	}
	for _, upd := range cand.updates {
		err = d.validateLeafRef(ctx, resTree, upd)
		if err != nil {
			return err
		}
	}

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
	// this semaphore controls the number of concurrent writes to the tree
	// this can be made a config knob, number of concurrent writes
	sem := semaphore.NewWeighted(1)
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
			d.storeSyncMsg(ctx, syncup, sem)
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

func (d *Datastore) validateLeafRef(ctx context.Context, t *ctree.Tree, upd *schemapb.Update) error {
	done := make(chan struct{})
	ch, err := d.getSchemaElements(ctx, upd.GetPath(), done)
	if err != nil {
		return err
	}
	defer close(done)
	//
	peIndex := 0
	var pe *schemapb.PathElem
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
			pe = upd.GetPath().GetElem()[peIndex]
			peIndex++
			switch sch := sch.Schema.(type) {
			case *schemapb.GetSchemaResponse_Container:
				for _, keySchema := range sch.Container.GetKeys() {
					if keySchema.GetType().GetType() != "leafref" {
						continue
					}
					leafRefPath, err := utils.StripPathElemPrefix(keySchema.GetType().GetLeafref())
					if err != nil {
						return err
					}
					log.Debugf("validating leafRef key %q: %q", keySchema.Name, leafRefPath)
					cp, err := utils.CompletePathFromString(leafRefPath)
					if err != nil {
						return err
					}
					pLen := len(cp)
					if pLen == 0 {
						return fmt.Errorf("could not determine reference path for ContainerKey %q: got %q", keySchema.Name, leafRefPath)
					}
					keyValue := pe.GetKey()[cp[pLen-1]]
					tr := t.Get(cp[:pLen-1])
					if tr == nil {
						return fmt.Errorf("missing leaf reference %q: %q", leafRefPath, keyValue)
					}
					if _, ok := tr.Children()[keyValue]; !ok {
						return fmt.Errorf("missing leaf reference %q: %q", leafRefPath, keyValue)
					}
				}
			case *schemapb.GetSchemaResponse_Field:
				if sch.Field.GetType().GetType() != "leafref" {
					continue
				}
				leafRefPath, err := utils.StripPathElemPrefix(sch.Field.GetType().GetLeafref())
				if err != nil {
					return err
				}
				cp, err := utils.CompletePathFromString(leafRefPath)
				if err != nil {
					return err
				}
				pLen := len(cp)
				if pLen == 0 {
					return fmt.Errorf("could not determine reference path for field %q: got %q", sch.Field.Name, leafRefPath)
				}
				tr := t.Get(cp[:pLen-1])
				if _, ok := tr.Children()[upd.GetValue().GetStringVal()]; !ok { // TODO: update when stored values are not stringVal anymore
					return fmt.Errorf("missing leaf reference %q: %q", leafRefPath, upd.GetValue().GetStringVal())
				}
			case *schemapb.GetSchemaResponse_Leaflist:
				if sch.Leaflist.GetType().GetType() != "leafref" {
					continue
				}
				leafRefPath, err := utils.StripPathElemPrefix(sch.Leaflist.GetType().GetLeafref())
				if err != nil {
					return err
				}
				fmt.Println("!! found leafref leaflist", sch.Leaflist.Name, leafRefPath)
			}
		}
	}
}

func (d *Datastore) storeSyncMsg(ctx context.Context, syncup *target.SyncUpdate, sem *semaphore.Weighted) {
	defer sem.Release(1)
	for _, del := range syncup.Update.GetDelete() {
		if d.config.Sync != nil && d.config.Sync.Validate {
			scRsp, err := d.getSchema(ctx, del)
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
			scRsp, err := d.getSchema(ctx, upd.GetPath())
			if err != nil {
				log.Errorf("datastore %s failed to get schema for update path %v: %v", d.config.Name, upd.GetPath(), err)
				continue
			}
			// workaround, skip presence containers
			switch r := scRsp.Schema.(type) {
			case *schemapb.GetSchemaResponse_Container:
				if r.Container.IsPresence {
					continue
				}
			}
			// _ = scRsp // TODO validate value
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

// helper for GetSchema
func (d *Datastore) getSchema(ctx context.Context, p *schemapb.Path) (*schemapb.GetSchemaResponse, error) {
	return d.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Path: p,
		Schema: &schemapb.Schema{
			Name:    d.Schema().Name,
			Vendor:  d.Schema().Vendor,
			Version: d.Schema().Version,
		},
	})
}

func (d *Datastore) getSchemaElements(ctx context.Context, p *schemapb.Path, done chan struct{}) (chan *schemapb.GetSchemaResponse, error) {
	stream, err := d.schemaClient.GetSchemaElements(ctx, &schemapb.GetSchemaRequest{
		Path: p,
		Schema: &schemapb.Schema{
			Name:    d.Schema().Name,
			Vendor:  d.Schema().Vendor,
			Version: d.Schema().Version,
		},
	})
	if err != nil {
		return nil, err
	}
	ch := make(chan *schemapb.GetSchemaResponse)
	go func() {
		defer close(ch)
		for {
			r, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				log.Errorf("GetSchemaElements stream err: %v", err)
				return
			}
			select {
			case <-done:
				return
			case ch <- r:
			}
		}
	}()
	return ch, nil
}
