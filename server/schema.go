package server

import (
	"context"
	"fmt"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
	"github.com/iptecharch/schema-server/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetSchema(ctx context.Context, req *schemapb.GetSchemaRequest) (*schemapb.GetSchemaResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	sc, ok := s.schemas[fmt.Sprintf("%s@%s@%s", reqSchema.Name, reqSchema.Vendor, reqSchema.Version)]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}

	pes := PathToStrings(req.GetPath())
	e, err := sc.GetEntry(pes)
	if err != nil {
		return nil, err
	}

	o := schema.ObjectFromYEntry(e)
	switch o := o.(type) {
	case *schemapb.ContainerSchema:
		return &schemapb.GetSchemaResponse{
			Schema: &schemapb.GetSchemaResponse_Container{
				Container: o,
			},
		}, nil
	case *schemapb.LeafListSchema:
		return &schemapb.GetSchemaResponse{
			Schema: &schemapb.GetSchemaResponse_Leaflist{
				Leaflist: o,
			},
		}, nil
	case *schemapb.LeafSchema:
		return &schemapb.GetSchemaResponse{
			Schema: &schemapb.GetSchemaResponse_Field{
				Field: o,
			},
		}, nil
	}
	return nil, fmt.Errorf("unknown schema item: %T", o)
}

func (s *Server) ListSchema(ctx context.Context, req *schemapb.ListSchemaRequest) (*schemapb.ListSchemaResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	rsp := &schemapb.ListSchemaResponse{
		Schema: make([]*schemapb.Schema, 0, len(s.schemas)),
	}
	for _, sc := range s.schemas {
		rsp.Schema = append(rsp.Schema,
			&schemapb.Schema{
				Name:    sc.Name(),
				Vendor:  sc.Vendor(),
				Version: sc.Version(),
			})
	}
	return rsp, nil
}

func (s *Server) GetSchemaDetails(ctx context.Context, req *schemapb.GetSchemaDetailsRequest) (*schemapb.GetSchemaDetailsResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	sc, ok := s.schemas[fmt.Sprintf("%s@%s@%s", reqSchema.Name, reqSchema.Vendor, reqSchema.Version)]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	rsp := &schemapb.GetSchemaDetailsResponse{
		Schema: &schemapb.Schema{
			Name:    sc.Name(),
			Vendor:  sc.Vendor(),
			Version: sc.Version(),
			Status:  0,
		},
		File:      sc.Files(),
		Directory: sc.Dirs(),
	}
	//
	return rsp, nil
}

func (s *Server) CreateSchema(ctx context.Context, req *schemapb.CreateSchemaRequest) (*schemapb.CreateSchemaResponse, error) {
	s.ms.RLock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		s.ms.RUnlock()
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	_, ok := s.schemas[fmt.Sprintf("%s@%s@%s", reqSchema.GetName(), reqSchema.GetVendor(), reqSchema.GetVersion())]
	s.ms.RUnlock()
	if ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v already exists", reqSchema)
	}
	switch {
	case req.GetSchema().GetName() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	sc, err := schema.NewSchema(
		&config.SchemaConfig{
			Name:        req.GetSchema().GetName(),
			Vendor:      req.GetSchema().GetVendor(),
			Version:     req.GetSchema().GetVersion(),
			Files:       req.GetFile(),
			Directories: req.GetDirectory(),
		},
	)
	if err != nil {
		return nil, err
	}

	// write
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[sc.UniqueName()] = sc
	scrsp := req.GetSchema()

	return &schemapb.CreateSchemaResponse{
		Schema: scrsp,
	}, nil
}

func (s *Server) ReloadSchema(ctx context.Context, req *schemapb.ReloadSchemaRequest) (*schemapb.ReloadSchemaResponse, error) {
	s.ms.RLock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		s.ms.RUnlock()
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	sc, ok := s.schemas[fmt.Sprintf("%s@%s@%s", reqSchema.GetName(), reqSchema.GetVendor(), reqSchema.GetVersion())]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	nsc, err := sc.Reload()
	if err != nil {
		return nil, err
	}
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[nsc.UniqueName()] = nsc
	return &schemapb.ReloadSchemaResponse{}, nil
}

func (s *Server) DeleteSchema(ctx context.Context, req *schemapb.DeleteSchemaRequest) (*schemapb.DeleteSchemaResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	case req.GetSchema().GetName() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	uniqueName := fmt.Sprintf("%s@%s@%s", reqSchema.GetName(), reqSchema.GetVendor(), reqSchema.GetVersion())
	s.ms.RLock()
	_, ok := s.schemas[uniqueName]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	s.ms.Lock()
	defer s.ms.Unlock()
	delete(s.schemas, uniqueName)
	return &schemapb.DeleteSchemaResponse{}, nil
}

func (s *Server) ToPath(ctx context.Context, req *schemapb.ToPathRequest) (*schemapb.ToPathResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	case req.GetSchema().GetName() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	uniqueName := fmt.Sprintf("%s@%s@%s", reqSchema.GetName(), reqSchema.GetVendor(), reqSchema.GetVersion())
	s.ms.RLock()
	sc, ok := s.schemas[uniqueName]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	p := &schemapb.Path{
		Elem: make([]*schemapb.PathElem, 0),
	}
	err := sc.BuildPath(req.GetPathElement(), p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	rsp := &schemapb.ToPathResponse{
		Path: p,
	}
	return rsp, nil
}

func (s *Server) ExpandPath(ctx context.Context, req *schemapb.ExpandPathRequest) (*schemapb.ExpandPathResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	case req.GetSchema().GetName() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	uniqueName := fmt.Sprintf("%s@%s@%s", reqSchema.GetName(), reqSchema.GetVendor(), reqSchema.GetVersion())
	s.ms.RLock()
	sc, ok := s.schemas[uniqueName]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	paths, err := sc.ExpandPath(req.GetPath(), req.GetDataType())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if req.GetXpath() {
		xpaths := make([]string, 0, len(paths))
		for _, p := range paths {
			xpaths = append(xpaths, utils.ToXPath(p, false))
		}
		rsp := &schemapb.ExpandPathResponse{
			Xpath: xpaths,
		}
		return rsp, nil
	}
	rsp := &schemapb.ExpandPathResponse{
		Path: paths,
	}
	return rsp, nil
}
