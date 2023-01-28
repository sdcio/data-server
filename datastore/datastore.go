package datastore

import (
	"context"
	"fmt"
	"sync"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	"github.com/iptecharch/schema-server/datastore/target"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// type Config struct {
// 	Name   string
// 	schema schemaRef
// }

// type schemaRef struct {
// 	Name    string
// 	Vendor  string
// 	Version string
// }

type Datastore struct {
	config     *config.DatastoreConfig
	main       *ctree.Tree
	m          *sync.RWMutex
	candidates map[string]*Candidate

	sbi          target.Target
	schemaClient schemapb.SchemaServerClient
}

func New(c *config.DatastoreConfig, schemaServer *config.SchemaServer) *Datastore {
	ds := &Datastore{
		config:     c,
		main:       &ctree.Tree{},
		m:          &sync.RWMutex{},
		candidates: map[string]*Candidate{},
		sbi:        nil,
	}
	return ds
}

type Candidate struct {
	base     *ctree.Tree
	updates  []*schemapb.Update
	replaces []*schemapb.Update
	deletes  []*schemapb.Path
	head     *ctree.Tree
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
		return status.Errorf(codes.InvalidArgument, "missing candidate name")
	}
	d.m.Lock()
	defer d.m.Unlock()
	cand, ok := d.candidates[name]
	if !ok {
		return fmt.Errorf("unknown candidate %s", name)
	}
	resTree, err := cand.base.Clone()
	if err != nil {
		return err
	}
	_ = resTree
	for _, upd := range cand.updates {
		err = resTree.AddUpdate(upd)
		if err != nil {
			return err
		}
	}
	// TODO: validate tree
	// push updates to sbi
	return nil
}

func (d *Datastore) Discard(ctx context.Context, req *schemapb.DiscardRequest) error { return nil } //TODO2
func (d *Datastore) NewCandidate(name string) error                                  { return nil } // TODO2

func (d *Datastore) DeleteCandidate(name string) error { return nil } // TODO2

func (d *Datastore) Stop() {} // todo2:
