package core

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore"
	logf "github.com/sdcio/logger"
)

// CreateDataStore creates and registers a datastore from spec. It fails with
// api.ErrInvalidArgument for a missing or oversized name, an existing name, an
// unknown target or sync protocol, or an invalid resulting config.
func (c *Core) CreateDataStore(ctx context.Context, spec api.DatastoreSpec) error {
	log := logf.FromContext(ctx).WithName("CreateDataStore").WithValues("datastore-name", spec.Name)

	if spec.Name == "" {
		return fmt.Errorf("%w: missing datastore name", api.ErrInvalidArgument)
	}
	if len(spec.Name) > math.MaxUint16 {
		return fmt.Errorf("%w: datastore name too long", api.ErrInvalidArgument)
	}
	if _, err := c.getDatastore(spec.Name); err == nil {
		return fmt.Errorf("%w: datastore %s already exists", api.ErrInvalidArgument, spec.Name)
	}

	dsCfg, err := c.datastoreConfig(spec)
	if err != nil {
		return err
	}
	if err := dsCfg.ValidateSetDefaults(); err != nil {
		return fmt.Errorf("%w: invalid datastore config: %v", api.ErrInvalidArgument, err)
	}

	ds, err := datastore.New(c.ctx, dsCfg, c.schemaClient, c.cacheClient, c.dialOpts...)
	if err != nil {
		return err
	}
	if err := c.addDatastore(ds); err != nil {
		_ = ds.Stop(ctx)
		return fmt.Errorf("%w: %v", api.ErrInvalidArgument, err)
	}
	log.Info("datastore created successfully")
	return nil
}

// DeleteDataStore stops the named datastore and removes it with its cached
// state. It fails with api.ErrInvalidArgument for an empty name and
// api.ErrUnknownDatastore when no such datastore exists.
func (c *Core) DeleteDataStore(ctx context.Context, name string) error {
	log := logf.FromContext(ctx).WithName("DeleteDataStore").WithValues("datastore-name", name)

	c.mu.Lock()
	ds, ok := c.datastores[name]
	if name != "" && ok {
		delete(c.datastores, name)
	}
	c.mu.Unlock()
	if name == "" {
		return fmt.Errorf("%w: missing datastore name", api.ErrInvalidArgument)
	}
	if !ok {
		return fmt.Errorf("%w %s", api.ErrUnknownDatastore, name)
	}

	if err := ds.Stop(c.ctx); err != nil {
		log.Error(err, "failed to stop datastore")
	}
	_ = ds.Delete(ctx)
	log.Info("deleted datastore")
	return nil
}

func (c *Core) datastoreConfig(spec api.DatastoreSpec) (*config.DatastoreConfig, error) {
	sbi, err := sbiFromTarget(&spec.Target)
	if err != nil {
		return nil, err
	}
	dsCfg := &config.DatastoreConfig{
		Name: spec.Name,
		Schema: &config.SchemaConfig{
			Vendor:  spec.Schema.Vendor,
			Version: spec.Schema.Version,
		},
		SBI:        sbi,
		Validation: c.cfg.Validation.DeepCopy(),
		Deviation:  c.cfg.Deviation.DeepCopy(),
	}
	dsCfg.Sync = &config.Sync{
		Validate:     spec.Sync.Validate,
		Buffer:       spec.Sync.Buffer,
		WriteWorkers: spec.Sync.WriteWorkers,
		Config:       make([]*config.SyncProtocol, 0, len(spec.Sync.Configs)),
	}
	for i := range spec.Sync.Configs {
		sp, err := syncProtocol(&spec.Sync.Configs[i])
		if err != nil {
			return nil, err
		}
		dsCfg.Sync.Config = append(dsCfg.Sync.Config, sp)
	}
	return dsCfg, nil
}

func sbiFromTarget(t *api.Target) (*config.SBI, error) {
	sbi := &config.SBI{Type: string(t.Type), Address: t.Address, Port: t.Port}
	switch strings.ToLower(string(t.Type)) {
	case "netconf":
		o := t.ProtocolOptions.Netconf
		if o == nil {
			o = &api.NetconfOptions{}
		}
		commit := "candidate"
		switch o.CommitCandidate {
		case api.CommitCandidateCandidate:
		case api.CommitCandidateRunning:
			commit = "running"
		default:
			return nil, fmt.Errorf("%w: unknown commit datastore %v", api.ErrInvalidArgument, o.CommitCandidate)
		}
		sbi.NetconfOptions = &config.SBINetconfOptions{
			IncludeNS:              o.IncludeNs,
			OperationWithNamespace: o.OperationWithNs,
			UseOperationRemove:     o.UseOperationRemove,
			CommitDatastore:        commit,
		}
	case "gnmi":
		g := t.ProtocolOptions.GNMI
		if g == nil {
			g = &api.GNMIOptions{}
		}
		sbi.GnmiOptions = &config.SBIGnmiOptions{Encoding: g.Encoding, TargetName: g.TargetName}
	case "noop":
	default:
		return nil, fmt.Errorf("%w: unknown target connection protocol type %q", api.ErrInvalidArgument, t.Type)
	}
	if t.TLS != nil {
		sbi.TLS = &config.TLS{CA: t.TLS.CA, Cert: t.TLS.Cert, Key: t.TLS.Key, SkipVerify: t.TLS.SkipVerify}
	}
	if t.Credentials != nil {
		sbi.Credentials = &config.Creds{Username: t.Credentials.Username, Password: t.Credentials.Password, Token: t.Credentials.Token}
	}
	return sbi, nil
}

func syncProtocol(sc *api.SyncConfig) (*config.SyncProtocol, error) {
	var typ string
	var gnmi *api.GNMIOptions
	if sc.Target != nil {
		typ = string(sc.Target.Type)
		gnmi = sc.Target.ProtocolOptions.GNMI
	}
	sp := &config.SyncProtocol{
		Protocol: typ,
		Name:     sc.Name,
		Paths:    sc.Paths,
		Interval: time.Duration(sc.Interval),
	}
	switch strings.ToLower(typ) {
	case "gnmi":
		sp.Mode = syncModeName(sc.Mode)
		if gnmi != nil {
			sp.Encoding = gnmi.Encoding
		}
	case "netconf":
	default:
		return nil, fmt.Errorf("%w: unknown target sync protocol %q", api.ErrInvalidArgument, typ)
	}
	return sp, nil
}

func syncModeName(m api.SyncMode) string {
	switch m {
	case api.SyncModeSample:
		return "sample"
	case api.SyncModeOnce:
		return "once"
	case api.SyncModeGet:
		return "get"
	default:
		return "on-change"
	}
}
