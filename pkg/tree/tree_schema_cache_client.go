package tree

import (
	"context"
	"strings"
	"sync"

	"github.com/sdcio/data-server/pkg/cache"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	PATHSEP = "/"
)

type TreeSchemaCacheClient interface {
	// CACHE based Functions
	// ReadIntended retrieves the highes priority value from the intended store
	Read(ctx context.Context, opts *cache.Opts, paths [][]string) []*cache.Update

	// SCHEMA based Functions
	GetSchema(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error)
	ToPath(ctx context.Context, path []string) (*sdcpb.Path, error)
}

type TreeSchemaCacheClientImpl struct {
	cc               cache.Client
	scb              SchemaClient.SchemaClientBound
	schemaIndex      map[string]*sdcpb.GetSchemaResponse
	schemaIndexMutex sync.RWMutex
	datastore        string
}

func NewTreeSchemaCacheClient(datastore string, cc cache.Client, scb SchemaClient.SchemaClientBound) *TreeSchemaCacheClientImpl {
	return &TreeSchemaCacheClientImpl{
		cc:          cc,
		scb:         scb,
		schemaIndex: map[string]*sdcpb.GetSchemaResponse{},
		datastore:   datastore,
	}
}

func (c *TreeSchemaCacheClientImpl) Read(ctx context.Context, opts *cache.Opts, paths [][]string) []*cache.Update {
	if opts == nil {
		opts = &cache.Opts{
			PriorityCount: 1,
		}
	}

	return c.cc.Read(ctx, c.datastore, opts, paths, 1)
}

// ToPath local implementation of the ToPath functinality. It takes a string slice that contains schema elements as well as key values.
// Via the help of the schema, the key elemens are being identified and an sdcpb.Path is returned.
func (c *TreeSchemaCacheClientImpl) ToPath(ctx context.Context, path []string) (*sdcpb.Path, error) {
	p := &sdcpb.Path{}
	// iterate through the path slice
	for i := 0; i < len(path); i++ {
		// create a PathElem for the actual index
		newPathElem := &sdcpb.PathElem{Name: path[i]}
		// append the path elem to the path
		p.Elem = append(p.Elem, newPathElem)
		// retrieve the schema
		schema, err := c.retrieveSchema(ctx, p)
		if err != nil {
			return nil, err
		}

		// break early if the container itself is defined in the path, not a sub-element
		if len(path) <= i+1 {
			break
		}

		// if it is a container with keys
		if schemaKeys := schema.GetSchema().GetContainer().GetKeys(); schemaKeys != nil {
			// add key map
			newPathElem.Key = make(map[string]string, len(schemaKeys))
			// adding the keys with the value from path[i], which is the key value
			for _, k := range schemaKeys {
				i++
				newPathElem.Key[k.Name] = path[i]
			}
		}
	}
	return p, nil
}

// retrieveSchema internal function to retrieve a schema, which when retireved will also be
// stored in the TreeSchemaCacheClientImpl's schema index
func (c *TreeSchemaCacheClientImpl) retrieveSchema(ctx context.Context, p *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {

	// convert the path into a keyless path, for schema index lookups.
	keylessPathSlice := utils.ToStrings(p, false, true)
	keylessPath := strings.Join(keylessPathSlice, PATHSEP)

	c.schemaIndexMutex.RLock()
	// first check if it was inserted meanwhile
	v, exists := c.schemaIndex[keylessPath]
	c.schemaIndexMutex.RUnlock()
	// if so, return it.
	if exists {
		return v, nil
	}

	c.schemaIndexMutex.Lock()
	defer c.schemaIndexMutex.Unlock()

	// try to see again if it exists, some other goroutine might have inserted in the meantime
	v, exists = c.schemaIndex[keylessPath]
	// if so, return it.
	if exists {
		return v, nil
	}

	// if schema wasn't found in index, go and fetch it
	schemaRsp, err := c.scb.GetSchema(ctx, p)
	if err != nil {
		return nil, err
	}

	// store the schema in the lookup index
	c.schemaIndex[keylessPath] = schemaRsp
	return schemaRsp, nil
}

// GetSchema retrieves the given schema element from the schema-server.
// relies on TreeSchemaCacheClientImpl.retrieveSchema(...) to source the internal lookup index (cache) of schemas
func (c *TreeSchemaCacheClientImpl) GetSchema(ctx context.Context, path []string) (*sdcpb.GetSchemaResponse, error) {
	// convert the []string path into sdcpb.path for schema retrieval
	sdcpbPath, err := c.ToPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.retrieveSchema(ctx, sdcpbPath)
}
