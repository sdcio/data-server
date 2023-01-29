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
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Datastore struct {
	config     *config.DatastoreConfig
	main       *ctree.Tree
	m          *sync.RWMutex
	candidates map[string]*Candidate

	sbi          target.Target
	schemaClient schemapb.SchemaServerClient

	cfn context.CancelFunc
}

func New(c *config.DatastoreConfig, schemaServer *config.SchemaServer) *Datastore {
	ds := &Datastore{
		config:     c,
		main:       &ctree.Tree{},
		m:          &sync.RWMutex{},
		candidates: map[string]*Candidate{},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	ds.cfn = cancel
	go func() {
	SCHEMA_CONNECT:
		// TODO: create grpc client and schema client
		cc, err := grpc.DialContext(ctx, schemaServer.Address,
			grpc.WithBlock(),
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			),
		)
		if err != nil {
			logrus.Errorf("failed to connect DS to schema server :%v", err)
			time.Sleep(time.Second)
			goto SCHEMA_CONNECT
		}
		ds.schemaClient = schemapb.NewSchemaServerClient(cc)
	}()
	var err error

	go func() {
	CONNECT:
		ds.sbi, err = target.New(ctx, c.Name, c.SBI, ds.main)
		if err != nil {
			logrus.Errorf("failed to create DS target :%v", err)
			time.Sleep(time.Second)
			goto CONNECT
		}
	}()
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

func (d *Datastore) Discard(ctx context.Context, req *schemapb.DiscardRequest) error {
	d.m.Lock()
	defer d.m.Unlock()
	cand, ok := d.candidates[req.GetDatastore().GetName()]
	if !ok {
		return fmt.Errorf("unknown candidate %s", req.GetDatastore().GetName())
	}

	cand.updates = make([]*schemapb.Update, 0)
	cand.replaces = make([]*schemapb.Update, 0)
	cand.deletes = make([]*schemapb.Path, 0)
	return nil
}

func (d *Datastore) NewCandidate(name string) error {
	d.m.Lock()
	defer d.m.Unlock()
	base, err := d.main.Clone()
	if err != nil {
		return err
	}
	d.candidates[name] = &Candidate{
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
