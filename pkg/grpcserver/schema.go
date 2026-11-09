package grpcserver

import (
	"context"
	"strings"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/schema"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Unary schema RPCs run behind the ready gate, so the core's schema client is
// set; the streaming RPCs are not gated and use streamSchema instead.

func schemaValues(s *sdcpb.Schema) []any {
	return []any{"schema-name", s.GetName(), "schema-vendor", s.GetVendor(), "schema-version", s.GetVersion()}
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	ctx, log := begin(ctx, "GetSchemaDetails", req, schemaValues(req.GetSchema())...)
	log.Info("GetSchemaDetails")
	info, err := s.core.GetSchemaDetails(ctx, schemaRef(req.GetSchema()))
	if err != nil {
		return nil, err
	}
	return schemaDetails(info), nil
}

func (s *Server) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	ctx, log := begin(ctx, "CreateSchema", req, schemaValues(req.GetSchema())...)
	log.Info("CreateSchema")
	err := s.core.CreateSchema(ctx, api.CreateSchemaSpec{
		Schema:      schemaRef(req.GetSchema()),
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
	ctx, log := begin(ctx, "DeleteSchema", req, schemaValues(req.GetSchema())...)
	log.Info("DeleteSchema")
	if err := s.core.DeleteSchema(ctx, api.DeleteSchemaSpec{Schema: schemaRef(req.GetSchema())}); err != nil {
		return nil, err
	}
	return &sdcpb.DeleteSchemaResponse{}, nil
}

func (s *Server) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	ctx, log := begin(ctx, "GetSchema", req, schemaValues(req.GetSchema())...)
	log.Info("GetSchema",
		"schema-path", req.GetPath().String(),
		"schema-validate-keys", req.GetValidateKeys(),
		"schema-with-description", req.GetWithDescription(),
	)
	return s.core.SchemaClient().GetSchema(ctx, req)
}

func (s *Server) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	ctx, _ = begin(ctx, "ListSchema", req)
	return s.core.SchemaClient().ListSchema(ctx, req)
}

func (s *Server) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	ctx, log := begin(ctx, "ReloadSchema", req, schemaValues(req.GetSchema())...)
	log.Info("ReloadSchema")
	return s.core.SchemaClient().ReloadSchema(ctx, req)
}

func (s *Server) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	ctx, log := begin(ctx, "ToPath", req, schemaValues(req.GetSchema())...)
	log.Info("ToPath", "path-elements", strings.Join(req.GetPathElement(), ","))
	return s.core.SchemaClient().ToPath(ctx, req)
}

func (s *Server) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	ctx, log := begin(ctx, "ExpandPath", req, schemaValues(req.GetSchema())...)
	log.Info("ExpandPath",
		"schema-path", req.GetPath().String(),
		"schema-datatype", req.GetDataType().String(),
		"schema-is-xpath", req.GetXpath(),
	)
	return s.core.SchemaClient().ExpandPath(ctx, req)
}

// streamSchema returns the schema client, or Unavailable while the core is
// still starting.
func (s *Server) streamSchema() (schema.Client, error) {
	c := s.core.SchemaClient()
	if c == nil || !s.ready.Load() {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return c, nil
}

func (s *Server) UploadSchema(stream sdcpb.SchemaServer_UploadSchemaServer) error {
	sc, err := s.streamSchema()
	if err != nil {
		return err
	}
	up, err := sc.UploadSchema(stream.Context())
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if err := up.Send(msg); err != nil {
			return err
		}
	}
}

func (s *Server) GetSchemaElements(req *sdcpb.GetSchemaRequest, stream sdcpb.SchemaServer_GetSchemaElementsServer) error {
	sc, err := s.streamSchema()
	if err != nil {
		return err
	}
	ctx, log := begin(stream.Context(), "GetSchemaElements", req, schemaValues(req.GetSchema())...)
	log.Info("GetSchemaElements",
		"schema-path", req.GetPath().String(),
		"schema-validate-keys", req.GetValidateKeys(),
		"schema-with-description", req.GetWithDescription(),
	)
	ch, err := sc.GetSchemaElements(ctx, req)
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
