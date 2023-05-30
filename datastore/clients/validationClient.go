package clients

import (
	"github.com/iptecharch/data-server/cache"
	CacheClient "github.com/iptecharch/data-server/datastore/clients/cache"
	SchemaClient "github.com/iptecharch/data-server/datastore/clients/schema"
	"github.com/iptecharch/schema-server/protos/schema_server"
)

type ValidationClient struct {
	*CacheClient.CacheClientBound
	*SchemaClient.SchemaClientBound
}

func NewValidationClient(datastoreName string, c cache.Client, s *schema_server.Schema, sc schema_server.SchemaServerClient) *ValidationClient {
	return &ValidationClient{
		CacheClientBound:  CacheClient.NewCacheClientBound(datastoreName, c),
		SchemaClientBound: SchemaClient.NewSchemaClientBound(s, sc),
	}
}
