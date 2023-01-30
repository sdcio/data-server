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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Datastore struct {
	config     *config.DatastoreConfig
	main       *ctree.Tree
	state      *ctree.Tree
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
		state:      &ctree.Tree{},
		m:          &sync.RWMutex{},
		candidates: map[string]*Candidate{},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	ds.cfn = cancel
	go func() {
	SCHEMA_CONNECT:
		opts := []grpc.DialOption{
			grpc.WithBlock(),
		}
		switch schemaServer.TLS {
		case nil:
			opts = append(opts,
				grpc.WithTransportCredentials(
					insecure.NewCredentials(),
				))
		default:
			tlsCfg, err := schemaServer.TLS.NewConfig(ctx)
			if err != nil {
				log.Errorf("DS: %s: failed to read schema server TLS config: %v", c.Name, err)
				time.Sleep(time.Second)
				goto SCHEMA_CONNECT
			}
			opts = append(opts,
				grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
			)
		}
		cc, err := grpc.DialContext(ctx, schemaServer.Address, opts...)
		if err != nil {
			log.Errorf("failed to connect DS to schema server :%v", err)
			time.Sleep(time.Second)
			goto SCHEMA_CONNECT
		}
		ds.schemaClient = schemapb.NewSchemaServerClient(cc)
	}()

	go func() {
		var err error
	CONNECT:
		ds.sbi, err = target.New(ctx, c.Name, c.SBI, ds.main)
		if err != nil {
			log.Errorf("failed to create DS target: %v", err)
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
	if req.GetRebase() {
		cand.base = d.main
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

	// TODO: 1. validate resTree
	// TODO: 1.1 validate added/removed leafrefs ?

	// push updates to sbi
	sbiSet := &schemapb.SetDataRequest{
		Update:  cand.updates,
		Replace: cand.replaces,
		Delete:  cand.deletes,
	}
	rsp, err := d.sbi.Set(ctx, sbiSet)
	if err != nil {
		return err
	}
	log.Debugf("DS=%s/%s, SetResponse from SBI: %v", d.config.Name, name, rsp)
	if req.GetStay() {
		return nil
	}
	delete(d.candidates, name)
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
