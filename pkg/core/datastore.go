package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

var errUnknownDatastore = api.ErrUnknownDatastore

// GetDataStore returns the observed state of the named datastore. It fails
// with api.ErrInvalidArgument for an empty name and api.ErrUnknownDatastore
// when no such datastore exists.
func (c *Core) GetDataStore(ctx context.Context, name string) (*api.DatastoreInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: missing datastore name", api.ErrInvalidArgument)
	}
	ds, err := c.getDatastore(name)
	if err != nil {
		return nil, err
	}
	return datastoreInfo(ctx, ds)
}

// WatchDeviations returns a channel of deviation events for the named
// datastore. The channel closes when ctx is done or the Core stops. Events are
// not produced yet; the datastore deviation feed is wired in a later step.
func (c *Core) WatchDeviations(ctx context.Context, name string) (<-chan api.DeviationEvent, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: missing datastore name", api.ErrInvalidArgument)
	}
	if _, err := c.getDatastore(name); err != nil {
		return nil, err
	}
	ch := make(chan api.DeviationEvent, deviationBuffer)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
		case <-c.ctx.Done():
		}
	}()
	return ch, nil
}

// deviationBuffer is the fixed capacity of a deviation watch channel.
const deviationBuffer = 8

func datastoreInfo(ctx context.Context, ds *datastore.Datastore) (*api.DatastoreInfo, error) {
	intents, err := ds.IntentsList(ctx)
	if err != nil {
		return nil, err
	}
	state := ds.ConnectionState()
	return &api.DatastoreInfo{
		Name:            ds.Name(),
		ConnectionState: connectionState(state.Status),
		StatusDetails:   state.Details,
		Intents:         intents,
		Spec:            specFromConfig(ds.Config()),
	}, nil
}

func connectionState(s sdcpb.TargetStatus) api.ConnectionState {
	switch s {
	case sdcpb.TargetStatus_CONNECTED:
		return api.ConnectionConnected
	case sdcpb.TargetStatus_NOT_CONNECTED:
		return api.ConnectionNotConnected
	default:
		return api.ConnectionUnknown
	}
}

func specFromConfig(dc *config.DatastoreConfig) api.DatastoreSpec {
	spec := api.DatastoreSpec{Name: dc.Name}
	if dc.Schema != nil {
		spec.Schema = api.SchemaRef{Vendor: dc.Schema.Vendor, Version: dc.Schema.Version}
	}
	if dc.SBI != nil {
		spec.Target = targetFromSBI(dc.SBI)
	}
	if dc.Sync != nil {
		for _, sp := range dc.Sync.Config {
			spec.Sync.Configs = append(spec.Sync.Configs, api.SyncConfig{
				Name:     sp.Name,
				Target:   &api.Target{Type: api.TargetType(strings.ToLower(sp.Protocol)), ProtocolOptions: api.ProtocolOptions{GNMI: gnmiSyncOptions(sp)}},
				Paths:    sp.Paths,
				Mode:     syncMode(sp.Mode),
				Interval: uint64(sp.Interval / time.Nanosecond),
			})
		}
	}
	return spec
}

func gnmiSyncOptions(sp *config.SyncProtocol) *api.GNMIOptions {
	if !strings.EqualFold(sp.Protocol, "gnmi") {
		return nil
	}
	return &api.GNMIOptions{Encoding: api.GNMIEncoding(sp.Encoding)}
}

func syncMode(m string) api.SyncMode {
	switch m {
	case "sample":
		return api.SyncModeSample
	case "once":
		return api.SyncModeOnce
	case "get":
		return api.SyncModeGet
	default:
		return api.SyncModeOnChange
	}
}

func targetFromSBI(s *config.SBI) api.Target {
	t := api.Target{
		Type:    api.TargetType(strings.ToLower(s.Type)),
		Address: s.Address,
		Port:    s.Port,
	}
	if s.Credentials != nil {
		t.Credentials = &api.Credentials{Username: s.Credentials.Username, Password: s.Credentials.Password, Token: s.Credentials.Token}
	}
	if s.TLS != nil {
		t.TLS = &api.TLS{CA: s.TLS.CA, Cert: s.TLS.Cert, Key: s.TLS.Key, SkipVerify: s.TLS.SkipVerify}
	}
	if s.GnmiOptions != nil {
		t.ProtocolOptions.GNMI = &api.GNMIOptions{Encoding: api.GNMIEncoding(s.GnmiOptions.Encoding), TargetName: s.GnmiOptions.TargetName}
	}
	if o := s.NetconfOptions; o != nil {
		cc := api.CommitCandidateCandidate
		if o.CommitDatastore == "running" {
			cc = api.CommitCandidateRunning
		}
		t.ProtocolOptions.Netconf = &api.NetconfOptions{
			IncludeNs:          o.IncludeNS,
			OperationWithNs:    o.OperationWithNamespace,
			UseOperationRemove: o.UseOperationRemove,
			CommitCandidate:    cc,
		}
	}
	return t
}
