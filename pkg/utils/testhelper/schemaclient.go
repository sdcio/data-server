package testhelper

import (
	"context"
	"fmt"

	dConfig "github.com/sdcio/data-server/pkg/config"
	dataschema "github.com/sdcio/data-server/pkg/schema"
	sConfig "github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	schemaStore "github.com/sdcio/schema-server/pkg/store"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
)

var (
	SDCIO_SCHEMA_LOCATION = "../../tests/schema"
)

type SchemaClient struct {
	schemaStore.Store
}

// returns schema name, vendor, version, and files path(s)
func (s *SchemaClient) GetSchemaDetails(ctx context.Context, in *sdcpb.GetSchemaDetailsRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaDetailsResponse, error) {
	return s.Store.GetSchemaDetails(ctx, in)
}

// lists known schemas with name, vendor, version and status
func (s *SchemaClient) ListSchema(ctx context.Context, in *sdcpb.ListSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ListSchemaResponse, error) {
	return s.Store.ListSchema(ctx, in)
}

// returns the schema of an item identified by a gNMI-like path
func (s *SchemaClient) GetSchema(ctx context.Context, in *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (*sdcpb.GetSchemaResponse, error) {
	return s.Store.GetSchema(ctx, in)
}

// creates a schema
func (s *SchemaClient) CreateSchema(ctx context.Context, in *sdcpb.CreateSchemaRequest, opts ...grpc.CallOption) (*sdcpb.CreateSchemaResponse, error) {
	return s.Store.CreateSchema(ctx, in)
}

// trigger schema reload
func (s *SchemaClient) ReloadSchema(ctx context.Context, in *sdcpb.ReloadSchemaRequest, opts ...grpc.CallOption) (*sdcpb.ReloadSchemaResponse, error) {
	return s.Store.ReloadSchema(ctx, in)
}

// delete a schema
func (s *SchemaClient) DeleteSchema(ctx context.Context, in *sdcpb.DeleteSchemaRequest, opts ...grpc.CallOption) (*sdcpb.DeleteSchemaResponse, error) {
	return s.Store.DeleteSchema(ctx, in)
}
func (s *SchemaClient) UploadSchema(ctx context.Context, opts ...grpc.CallOption) (sdcpb.SchemaServer_UploadSchemaClient, error) {
	return nil, fmt.Errorf("unimplemented")
}

// ToPath converts a list of items into a schema.proto.Path
func (s *SchemaClient) ToPath(ctx context.Context, in *sdcpb.ToPathRequest, opts ...grpc.CallOption) (*sdcpb.ToPathResponse, error) {
	return s.Store.ToPath(ctx, in)
}

// ExpandPath returns a list of sub paths given a single path
func (s *SchemaClient) ExpandPath(ctx context.Context, in *sdcpb.ExpandPathRequest, opts ...grpc.CallOption) (*sdcpb.ExpandPathResponse, error) {
	return s.Store.ExpandPath(ctx, in)
}

func (s *SchemaClient) GetSchemaElements(ctx context.Context, req *sdcpb.GetSchemaRequest, opts ...grpc.CallOption) (chan *sdcpb.SchemaElem, error) {
	return s.Store.GetSchemaElements(ctx, req)
}

func InitSDCIOSchema() (dataschema.Client, *dConfig.SchemaConfig, error) {

	// create an in memory schema store
	schemaMemStore := memstore.New()

	// define the schema config
	sc := &sConfig.SchemaConfig{
		Name:    "testschema",
		Vendor:  "sdcio",
		Version: "v0.0.0",
		Files: []string{
			SDCIO_SCHEMA_LOCATION,
		},
	}

	dsc := &dConfig.SchemaConfig{
		Name:    sc.Name,
		Vendor:  sc.Vendor,
		Version: sc.Version,
	}

	// init new schema definition to be read by the schema component
	schema, err := schema.NewSchema(sc)
	if err != nil {
		return nil, nil, err
	}

	// add the schema to the in memory schema store
	err = schemaMemStore.AddSchema(schema)
	if err != nil {
		return nil, nil, err
	}

	return &SchemaClient{Store: schemaMemStore}, dsc, nil
}
