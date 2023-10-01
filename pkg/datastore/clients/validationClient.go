package clients

import (
	schema_server "github.com/iptecharch/sdc-protos/sdcpb"

	"github.com/iptecharch/data-server/pkg/cache"
	CacheClient "github.com/iptecharch/data-server/pkg/datastore/clients/cache"
	SchemaClient "github.com/iptecharch/data-server/pkg/datastore/clients/schema"
	"github.com/iptecharch/data-server/pkg/schema"
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
