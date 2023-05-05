package datastore

import (
	"context"

	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/schema-server/cache"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
)

type MustValidationClientImpl struct {
	cacheClient  cache.Client
	schema       *schemapb.Schema
	schemaClient schemapb.SchemaServerClient
	name         string
}

func NewMustValidationClientImpl(name string, c cache.Client, s *schemapb.Schema, sc schemapb.SchemaServerClient) *MustValidationClientImpl {
	return &MustValidationClientImpl{
		cacheClient:  c,
		schema:       s,
		schemaClient: sc,
		name:         name, // the datastore name
	}
}

// GetSchema retrieves the schema for the given path
func (vc *MustValidationClientImpl) GetSchema(ctx context.Context, path *schemapb.Path) (*schemapb.GetSchemaResponse, error) {
	return vc.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Schema: &schemapb.Schema{
			Name:    vc.schema.Name,
			Version: vc.schema.Version,
			Vendor:  vc.schema.Vendor,
		},
		Path: path,
	})
}

// GetValue retrieves the value for the provided path
func (vc *MustValidationClientImpl) GetValue(ctx context.Context, path *schemapb.Path) (*schemapb.TypedValue, error) {
	spath, err := utils.CompletePath(nil, path)
	if err != nil {
		return nil, err
	}
	cacheupds := vc.cacheClient.Read(ctx, vc.name, cachepb.Store_CONFIG, [][]string{spath})
	if len(cacheupds) == 0 {
		return nil, nil
	}
	return cacheupds[0].Value()
}
