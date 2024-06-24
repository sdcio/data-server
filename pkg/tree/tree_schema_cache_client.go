package tree

import (
	"context"
	"strings"

	"github.com/sdcio/data-server/pkg/cache"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type TreeSchemaCacheClient interface {
	// CACHE based Functions
	// ReadIntended retrieves the highes priority value from the intended store
	Read(ctx context.Context, opts *cache.Opts, paths PathsSlice) []*cache.Update

	// SCHEMA based Functions
	GetSchema(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error)
	ToPath(ctx context.Context, path []string) (*sdcpb.Path, error)
}

type TreeSchemaCacheClientImpl struct {
	cc          cache.Client
	scb         SchemaClient.SchemaClientBound
	schemaIndex map[string]*sdcpb.GetSchemaResponse
	datastore   string
}

func NewTreeSchemaCacheClient(datastore string, cc cache.Client, scb SchemaClient.SchemaClientBound) *TreeSchemaCacheClientImpl {
	return &TreeSchemaCacheClientImpl{
		cc:          cc,
		scb:         scb,
		schemaIndex: map[string]*sdcpb.GetSchemaResponse{},
		datastore:   datastore,
	}
}

func (c *TreeSchemaCacheClientImpl) Read(ctx context.Context, opts *cache.Opts, paths PathsSlice) []*cache.Update {
	if opts == nil {
		opts = &cache.Opts{
			PriorityCount: 1,
		}
	}

	return c.cc.Read(ctx, c.datastore, opts, paths, 1)
}

func (c *TreeSchemaCacheClientImpl) ToPath(ctx context.Context, path []string) (*sdcpb.Path, error) {
	return c.scb.ToPath(ctx, path)
}

func (c *TreeSchemaCacheClientImpl) GetSchema(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error) {
	// convert the []string path into sdcpb.path for schema retrieval
	sdcpbPath, err := c.scb.ToPath(ctx, path)
	if err != nil {
		return nil, err
	}

	// convert the path into a keyless path, for schema index lookups.
	keylessPathSlice := utils.ToStrings(sdcpbPath, false, true)
	keylessPath := strings.Join(keylessPathSlice, "/")

	// lookup schema in schemaindex, preventing consecutive gets from the schema server
	if v, exists := c.schemaIndex[keylessPath]; exists {
		return v, nil
	}

	// if schema wasn't found in index, go and fetch it
	schemaRsp, err := c.scb.GetSchema(ctx, sdcpbPath)
	if err != nil {
		return nil, err
	}

	return schemaRsp, nil
}
