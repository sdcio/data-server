package core

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// GetSchemaDetails returns the state and source locations of a schema.
func (c *Core) GetSchemaDetails(ctx context.Context, ref api.SchemaRef) (*api.SchemaInfo, error) {
	ctx = logf.IntoContext(ctx, logf.FromContext(ctx).WithName("GetSchemaDetails").WithValues("schema-vendor", ref.Vendor, "schema-version", ref.Version))
	rsp, err := c.schemaClient.GetSchemaDetails(ctx, &sdcpb.GetSchemaDetailsRequest{Schema: toSdcpbSchema(ref)})
	if err != nil {
		return nil, err
	}
	return &api.SchemaInfo{
		Schema:      api.SchemaRef{Vendor: rsp.GetSchema().GetVendor(), Version: rsp.GetSchema().GetVersion()},
		State:       schemaState(rsp.GetSchema().GetStatus()),
		Files:       rsp.GetFile(),
		Directories: rsp.GetDirectory(),
		Excludes:    rsp.GetExclude(),
	}, nil
}

// CreateSchema loads a schema from the given files and directories.
func (c *Core) CreateSchema(ctx context.Context, spec api.CreateSchemaSpec) error {
	ctx = logf.IntoContext(ctx, logf.FromContext(ctx).WithName("CreateSchema").WithValues("schema-vendor", spec.Schema.Vendor, "schema-version", spec.Schema.Version))
	_, err := c.schemaClient.CreateSchema(ctx, &sdcpb.CreateSchemaRequest{
		Schema:    toSdcpbSchema(spec.Schema),
		File:      spec.Files,
		Directory: spec.Directories,
		Exclude:   spec.Excludes,
	})
	return err
}

// DeleteSchema removes a schema.
func (c *Core) DeleteSchema(ctx context.Context, spec api.DeleteSchemaSpec) error {
	ctx = logf.IntoContext(ctx, logf.FromContext(ctx).WithName("DeleteSchema").WithValues("schema-vendor", spec.Schema.Vendor, "schema-version", spec.Schema.Version))
	_, err := c.schemaClient.DeleteSchema(ctx, &sdcpb.DeleteSchemaRequest{Schema: toSdcpbSchema(spec.Schema)})
	return err
}

func toSdcpbSchema(ref api.SchemaRef) *sdcpb.Schema {
	return &sdcpb.Schema{Vendor: ref.Vendor, Version: ref.Version}
}

func schemaState(s sdcpb.SchemaStatus) api.SchemaState {
	switch s {
	case sdcpb.SchemaStatus_RELOADING:
		return api.SchemaReloading
	case sdcpb.SchemaStatus_INITIALIZING:
		return api.SchemaInitializing
	case sdcpb.SchemaStatus_FAILED:
		return api.SchemaFailed
	default:
		return api.SchemaReady
	}
}
