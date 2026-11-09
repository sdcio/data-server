package core

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/schema"
)

// ListDataStores returns the observed state of every registered datastore, in
// an undefined order.
func (c *Core) ListDataStores(ctx context.Context) ([]*api.DatastoreInfo, error) {
	c.mu.RLock()
	names := make([]string, 0, len(c.datastores))
	for n := range c.datastores {
		names = append(names, n)
	}
	c.mu.RUnlock()

	out := make([]*api.DatastoreInfo, 0, len(names))
	for _, n := range names {
		info, err := c.GetDataStore(ctx, n)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

// ListIntents returns intent names in the named datastore.
func (c *Core) ListIntents(ctx context.Context, datastore string) ([]string, error) {
	info, err := c.GetDataStore(ctx, datastore)
	if err != nil {
		return nil, err
	}
	return info.Intents, nil
}

// SchemaClient returns the schema client initialized by Start. It is nil until
// Start completes schema wiring.
func (c *Core) SchemaClient() schema.Client {
	return c.schemaClient
}
