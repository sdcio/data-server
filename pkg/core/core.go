// Package core is the in-process data-server: it owns the schema store, cache
// and datastores, and exposes them through the domain types of pkg/api.
package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore"
	"github.com/sdcio/data-server/pkg/schema"
	logf "github.com/sdcio/logger"
	"google.golang.org/grpc"
)

// Core is the data-server engine. Create it with New, run Start once, wait for
// readiness with WaitReady and release it with Stop. It needs no gRPC listener
// or peer.
type Core struct {
	cfg      *config.Config
	dialOpts []grpc.DialOption

	ctx    context.Context
	cancel context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once
	ready     chan struct{}
	readyErr  error

	schemaClient schema.Client
	cacheClient  cache.Client

	mu         sync.RWMutex
	datastores map[string]*datastore.Datastore
}

// Option customises a Core at construction.
type Option func(*Core)

// WithDialOptions sets the gRPC dial options used for southbound gNMI targets.
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(c *Core) { c.dialOpts = append(c.dialOpts, opts...) }
}

// New builds a Core from a validated config. It starts no work; the Core's
// lifetime is bounded by ctx and Stop.
func New(ctx context.Context, cfg *config.Config, opts ...Option) (*Core, error) {
	if cfg == nil {
		return nil, errors.New("core: nil config")
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &Core{
		cfg:        cfg,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
		datastores: map[string]*datastore.Datastore{},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Start initialises the schema store, the cache and the configured datastores,
// then marks the Core ready. It returns when initialisation finished or failed
// and only the first call does work; a failure is also reported by WaitReady.
// Any configured datastore that cannot be created fails Start.
func (c *Core) Start(ctx context.Context) error {
	c.startOnce.Do(func() {
		c.readyErr = c.start(ctx)
		close(c.ready)
	})
	<-c.ready
	return c.readyErr
}

// WaitReady blocks until Start completed, returning its error, or until ctx is
// done.
func (c *Core) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return c.readyErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops all datastores and cancels the Core's context. It is idempotent
// and always returns nil: datastore stop errors are discarded. A Start after
// Stop fails with context.Canceled.
func (c *Core) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, ds := range c.datastores {
			_ = ds.Stop(ctx)
		}
		c.cancel()
	})
	return nil
}

func (c *Core) start(ctx context.Context) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	log := logf.FromContext(ctx).WithName("data-server")
	ctx = logf.IntoContext(ctx, log)

	var wg sync.WaitGroup
	var schemaErr, cacheErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		schemaErr = c.createSchemaClient(ctx)
	}()
	go func() {
		defer wg.Done()
		cacheErr = c.createCacheClient(ctx)
	}()
	wg.Wait()
	if err := errors.Join(schemaErr, cacheErr); err != nil {
		return fmt.Errorf("core init: %w", err)
	}

	if err := c.createInitialDatastores(ctx); err != nil {
		return fmt.Errorf("core init: %w", err)
	}
	log.Info("data-server ready")
	return nil
}

// createInitialDatastores creates the datastores listed in the config and
// returns the joined errors of those that failed.
func (c *Core) createInitialDatastores(ctx context.Context) error {
	log := logf.FromContext(ctx)
	var wg sync.WaitGroup
	errs := make([]error, len(c.cfg.Datastores))
	for i, dsCfg := range c.cfg.Datastores {
		dsCfg.Validation = c.cfg.Validation.DeepCopy()
		wg.Add(1)
		go func(i int, dsCfg *config.DatastoreConfig) {
			defer wg.Done()
			ds, err := datastore.New(c.ctx, dsCfg, c.schemaClient, c.cacheClient, c.dialOpts...)
			if err == nil {
				err = c.addDatastore(ds)
			}
			if err != nil {
				errs[i] = fmt.Errorf("datastore %s: %w", dsCfg.Name, err)
			}
		}(i, dsCfg)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	log.Info("configured datastores initialized")
	return nil
}

func (c *Core) addDatastore(ds *datastore.Datastore) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.datastores[ds.Name()]; ok {
		return fmt.Errorf("datastore %s already exists", ds.Name())
	}
	c.datastores[ds.Name()] = ds
	return nil
}

// getDatastore fails with api.ErrInvalidArgument for an empty name and
// api.ErrUnknownDatastore when the name is not registered.
func (c *Core) getDatastore(name string) (*datastore.Datastore, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: missing datastore name", api.ErrInvalidArgument)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ds, ok := c.datastores[name]
	if !ok {
		return nil, fmt.Errorf("%w %s", api.ErrUnknownDatastore, name)
	}
	return ds, nil
}
