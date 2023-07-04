package schema

import (
	"context"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"google.golang.org/grpc"
)

type Client interface {
	// returns schema name, vendor, version, and files path(s)
	GetSchemaDetails(ctx context.Context, in *schemapb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*schemapb.GetSchemaDetailsResponse, error)
	// lists known schemas with name, vendor, version and status
	ListSchema(ctx context.Context, in *schemapb.ListSchemaRequest, opts ...grpc.CallOption) (*schemapb.ListSchemaResponse, error)
	// returns the schema of an item identified by a gNMI-like path
	GetSchema(ctx context.Context, in *schemapb.GetSchemaRequest, opts ...grpc.CallOption) (*schemapb.GetSchemaResponse, error)
	// creates a schema
	CreateSchema(ctx context.Context, in *schemapb.CreateSchemaRequest, opts ...grpc.CallOption) (*schemapb.CreateSchemaResponse, error)
	// trigger schema reload
	ReloadSchema(ctx context.Context, in *schemapb.ReloadSchemaRequest, opts ...grpc.CallOption) (*schemapb.ReloadSchemaResponse, error)
	// delete a schema
	DeleteSchema(ctx context.Context, in *schemapb.DeleteSchemaRequest, opts ...grpc.CallOption) (*schemapb.DeleteSchemaResponse, error)
	// client stream RPC to upload yang files to the server:
	// - uses CreateSchema as a first message
	// - then N intermediate UploadSchemaFile, initial, bytes, hash for each file
	// - and ends with an UploadSchemaFinalize{}
	UploadSchema(ctx context.Context, opts ...grpc.CallOption) (schemapb.SchemaServer_UploadSchemaClient, error)
	// ToPath converts a list of items into a schema.proto.Path
	ToPath(ctx context.Context, in *schemapb.ToPathRequest, opts ...grpc.CallOption) (*schemapb.ToPathResponse, error)
	// ExpandPath returns a list of sub paths given a single path
	ExpandPath(ctx context.Context, in *schemapb.ExpandPathRequest, opts ...grpc.CallOption) (*schemapb.ExpandPathResponse, error)

	// overwrites
	// TOOD: uploadSchema
	GetSchemaElements(ctx context.Context, req *schemapb.GetSchemaRequest, opts ...grpc.CallOption) (chan *schemapb.SchemaElem, error)
}
