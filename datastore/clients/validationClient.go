package clients

import (
	"github.com/iptecharch/data-server/cache"
	CacheClient "github.com/iptecharch/data-server/datastore/clients/cache"
	SchemaClient "github.com/iptecharch/data-server/datastore/clients/schema"
	"github.com/iptecharch/data-server/schema"
	schema_server "github.com/iptecharch/sdc-protos/sdcpb"
)

type ValidationClient struct {
	*CacheClient.CacheClientBound
	*SchemaClient.SchemaClientBound
}

func NewValidationClient(datastoreName string, c cache.Client, s *schema_server.Schema, sc schema.Client) *ValidationClient {
	return &ValidationClient{
		CacheClientBound:  CacheClient.NewCacheClientBound(datastoreName, c),
		SchemaClientBound: SchemaClient.NewSchemaClientBound(s, sc),
	}
}
