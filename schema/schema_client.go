package schema

import (
	"context"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"google.golang.org/grpc"
)

type Client interface {
	// returns schema name, vendor, version, and files path(s)
	GetSchemaDetails(ctx context.Context, in *sdcpb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaDetailsResponse, error)
	// lists known schemas with name, vendor, version and status
	ListSchema(ctx context.Context, in *sdcpb.ListSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ListSchemaResponse, error)
	// returns the schema of an item identified by a gNMI-like path
	GetSchema(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error)
	// creates a schema
	CreateSchema(ctx context.Context, in *sdcpb.CreateSchemaRequest, opts ...grpc.CallOption) (*sdcpb.CreateSchemaResponse, error)
	// trigger schema reload
	ReloadSchema(ctx context.Context, in *sdcpb.ReloadSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ReloadSchemaResponse, error)
	// delete a schema
	DeleteSchema(ctx context.Context, in *sdcpb.DeleteSchemaRequest, opts ...grpc.CallOption) (*sdcpb.DeleteSchemaResponse, error)
	// client stream RPC to upload yang files to the server:
	// - uses CreateSchema as a first message
	// - then N intermediate UploadSchemaFile, initial, bytes, hash for each file
	// - and ends with an UploadSchemaFinalize{}
	UploadSchema(ctx context.Context, opts ...grpc.CallOption) (sdcpb.SchemaServer_UploadSchemaClient, error)
	// ToPath converts a list of items into a schema.proto.Path
	ToPath(ctx context.Context, in *sdcpb.ToPathRequest, opts ...grpc.CallOption) (*sdcpb.ToPathResponse, error)
	// ExpandPath returns a list of sub paths given a single path
	ExpandPath(ctx context.Context, in *sdcpb.ExpandPathRequest, opts ...grpc.CallOption) (*sdcpb.ExpandPathResponse, error)

	// overwrites
	// TOOD: uploadSchema
	GetSchemaElements(ctx context.Context, req *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (chan *sdcpb.SchemaElem, error)
}
