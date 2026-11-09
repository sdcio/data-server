package grpcserver

import (
	"context"
	"strings"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	log := logf.FromContext(ctx).WithName("GetSchemaDetails").WithValues(
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("GetSchemaDetails")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	info, err := s.core.GetSchemaDetails(ctx, api.SchemaRef{Vendor: req.GetSchema().GetVendor(), Version: req.GetSchema().GetVersion()})
	if err != nil {
		return nil, err
	}
	return schemaDetailsToSdcpb(info), nil
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("CreateSchema").WithValues(
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("CreateSchema")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	err := s.core.CreateSchema(ctx, api.CreateSchemaSpec{
		Schema:      api.SchemaRef{Vendor: req.GetSchema().GetVendor(), Version: req.GetSchema().GetVersion()},
		Files:       req.GetFile(),
		Directories: req.GetDirectory(),
		Excludes:    req.GetExclude(),
	})
	if err != nil {
		return nil, err
	}
	return &sdcpb.CreateSchemaResponse{}, nil
}

func (s *Server) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	log := logf.FromContext(ctx).WithName("DeleteSchema").WithValues(
		"schema-vendor", req.GetSchema().GetVendor(),
		"schema-version", req.GetSchema().GetVersion(),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("DeleteSchema")
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if err := s.core.DeleteSchema(ctx, api.DeleteSchemaSpec{Schema: api.SchemaRef{Vendor: req.GetSchema().GetVendor(), Version: req.GetSchema().GetVersion()}}); err != nil {
		return nil, err
	}
	return &sdcpb.DeleteSchemaResponse{}, nil
}

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	if err := schemaClientOrError(s); err != nil {
		return nil, err
	}
	log := logf.FromContext(ctx).WithName("GetSchema")
	ctx = logf.IntoContext(ctx, log)
	return s.core.SchemaClient().GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	if err := schemaClientOrError(s); err != nil {
		return nil, err
	}
	log := logf.FromContext(ctx).WithName("ListSchema")
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))
	return s.core.SchemaClient().ListSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	if err := schemaClientOrError(s); err != nil {
		return nil, err
	}
	log := logf.FromContext(ctx).WithName("ReloadSchema")
	ctx = logf.IntoContext(ctx, log)
	return s.core.SchemaClient().ReloadSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	if err := schemaClientOrError(s); err != nil {
		return nil, err
	}
	log := logf.FromContext(ctx).WithName("ToPath")
	ctx = logf.IntoContext(ctx, log)
	log.Info("ToPath", "path-elements", strings.Join(req.GetPathElement(), ","))
	return s.core.SchemaClient().ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	if err := schemaClientOrError(s); err != nil {
		return nil, err
	}
	log := logf.FromContext(ctx).WithName("ExpandPath")
	ctx = logf.IntoContext(ctx, log)
	return s.core.SchemaClient().ExpandPath(ctx, req)
}

func (s *Server) UploadSchema(stream sdcpb.SchemaServer_UploadSchemaServer) error {
	if err := schemaClientOrError(s); err != nil {
		return err
	}
	ctx := stream.Context()
	client, err := s.core.SchemaClient().UploadSchema(ctx)
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if err := client.Send(msg); err != nil {
			return err
		}
	}
}

func (s *Server) GetSchemaElements(req *sdcpb.GetSchemaRequest, stream sdcpb.SchemaServer_GetSchemaElementsServer) error {
	if err := schemaClientOrError(s); err != nil {
		return err
	}
	ctx := stream.Context()
	ch, err := s.core.SchemaClient().GetSchemaElements(ctx, req)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sce, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&sdcpb.GetSchemaResponse{Schema: sce}); err != nil {
				return err
			}
		}
	}
}
