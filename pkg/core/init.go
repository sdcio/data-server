package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	cconfig "github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/schema"
	logf "github.com/sdcio/logger"
	schemaConfig "github.com/sdcio/schema-server/pkg/config"
	schemaServerSchema "github.com/sdcio/schema-server/pkg/schema"
	schemaStore "github.com/sdcio/schema-server/pkg/store"
	schemaMemoryStore "github.com/sdcio/schema-server/pkg/store/memstore"
	schemaPersistentStore "github.com/sdcio/schema-server/pkg/store/persiststore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (c *Core) createCacheClient(ctx context.Context) error {
	log := logf.FromContext(ctx).WithValues("cache-type", c.cfg.Cache.Type)
	if c.cfg.Cache.Type != "local" {
		return fmt.Errorf("unknown cache type: %s", c.cfg.Cache.Type)
	}
	log.Info("initializing local cache client", "store-type", c.cfg.Cache.StoreType, "directory", c.cfg.Cache.Dir)
	cl, err := cache.NewLocalCache(&cconfig.CacheConfig{
		StoreType: c.cfg.Cache.StoreType,
		Dir:       c.cfg.Cache.Dir,
	})
	if err != nil {
		return fmt.Errorf("local cache: %w", err)
	}
	c.cacheClient = cl
	log.Info("local cache created")
	return nil
}

func (c *Core) createSchemaClient(ctx context.Context) error {
	if c.cfg.SchemaStore != nil {
		return c.createLocalSchemaStore(ctx)
	}
	return c.createRemoteSchemaClient(ctx)
}

func (c *Core) createLocalSchemaStore(ctx context.Context) error {
	log := logf.FromContext(ctx)
	var store schemaStore.Store
	switch c.cfg.SchemaStore.Type {
	case schemaConfig.StoreTypeMemory:
		store = schemaMemoryStore.New()
	case schemaConfig.StoreTypePersistent:
		var err error
		store, err = schemaPersistentStore.New(ctx, c.cfg.SchemaStore.Path, c.cfg.SchemaStore.Cache, c.cfg.SchemaStore.ReadOnly)
		if err != nil {
			return fmt.Errorf("persistent schema store: %w", err)
		}
	default:
		return fmt.Errorf("unknown schema store type %q", c.cfg.SchemaStore.Type)
	}
	log.Info("parsing schema(s)...", "numschemas", len(c.cfg.SchemaStore.Schemas))

	var wg sync.WaitGroup
	for _, sCfg := range c.cfg.SchemaStore.Schemas {
		wg.Add(1)
		go func(sCfg *schemaConfig.SchemaConfig) {
			defer wg.Done()
			sck := schemaStore.SchemaKey{Name: sCfg.Name, Vendor: sCfg.Vendor, Version: sCfg.Version}
			log := log.WithValues("schema", sck)
			if store.HasSchema(sck) {
				log.Info("schema already exists in the store, not reloading it")
				return
			}
			sc, err := schemaServerSchema.NewSchema(sCfg)
			if err != nil {
				log.Error(err, "schema parsing failed")
				return
			}
			if err := store.AddSchema(sc); err != nil {
				log.Error(err, "failed to add schema to the store")
			}
		}(sCfg)
	}
	wg.Wait()
	c.schemaClient = schema.NewLocalClient(store)
	return nil
}

// createRemoteSchemaClient retries until the client is built or ctx is done.
func (c *Core) createRemoteSchemaClient(ctx context.Context) error {
	log := logf.FromContext(ctx).WithValues("schema-server-address", c.cfg.SchemaServer.Address)
	for {
		cc, err := c.dialSchemaServer(ctx)
		if err == nil {
			log.Info("connected to schema server")
			c.schemaClient = schema.NewRemoteClient(cc, c.cfg.SchemaServer.Cache)
			return nil
		}
		log.Error(err, "failed to create schema client")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (c *Core) dialSchemaServer(ctx context.Context) (*grpc.ClientConn, error) {
	creds := insecure.NewCredentials()
	if c.cfg.SchemaServer.TLS != nil {
		tlsCfg, err := c.cfg.SchemaServer.TLS.NewConfig(ctx)
		if err != nil {
			return nil, err
		}
		creds = credentials.NewTLS(tlsCfg)
	}
	return grpc.NewClient(c.cfg.SchemaServer.Address, grpc.WithTransportCredentials(creds))
}
