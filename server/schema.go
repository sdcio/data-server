package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"sort"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/schema"
	"github.com/iptecharch/schema-server/utils"
	log "github.com/sirupsen/logrus"
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
	pes := utils.ToStrings(req.GetPath(), false, true)
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
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	s.ms.RLock()
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
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	s.ms.RLock()
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
		sort.Strings(xpaths)
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

func (s *Server) UploadSchema(stream schemapb.SchemaServer_UploadSchemaServer) error {
	createReq, err := stream.Recv()
	if err != nil {
		return err
	}

	var name string
	var vendor string
	var version string
	var files []string
	var dirs []string

	switch req := createReq.Upload.(type) {
	case *schemapb.UploadSchemaRequest_CreateSchema:
		switch {
		case req.CreateSchema.GetSchema().GetName() == "":
			return status.Error(codes.InvalidArgument, "missing schema name")
		case req.CreateSchema.GetSchema().GetVendor() == "":
			return status.Error(codes.InvalidArgument, "missing schema vendor")
		case req.CreateSchema.GetSchema().GetVersion() == "":
			return status.Error(codes.InvalidArgument, "missing schema version")
		}
		name = req.CreateSchema.GetSchema().GetName()
		vendor = req.CreateSchema.GetSchema().GetVendor()
		version = req.CreateSchema.GetSchema().GetVersion()
		s.ms.RLock()
		_, ok := s.schemas[fmt.Sprintf("%s@%s@%s", name, vendor, version)]
		s.ms.RUnlock()
		if ok {
			return status.Errorf(codes.InvalidArgument, "schema %s/%s/%s already exists", name, vendor, version)
		}
	}
	dirname := fmt.Sprintf("%s_%s_%s", name, vendor, version)
	handledFiles := make(map[string]*os.File)
LOOP:
	for {
		updloadFileReq, err := stream.Recv()
		if err != nil {
			return err
		}
		switch updloadFileReq := updloadFileReq.Upload.(type) {
		case *schemapb.UploadSchemaRequest_SchemaFile:
			if updloadFileReq.SchemaFile.GetFileName() == "" {
				return status.Error(codes.InvalidArgument, "missing file name")
			}
			var uplFile *os.File
			var ok bool
			fileName := path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname, updloadFileReq.SchemaFile.GetFileName())
			if uplFile, ok = handledFiles[fileName]; !ok {
				osf, err := os.Create(fileName)
				if err != nil {
					return err
				}
				handledFiles[fileName] = osf
			}
			if len(updloadFileReq.SchemaFile.GetContents()) > 0 {
				_, err = uplFile.Write(updloadFileReq.SchemaFile.GetContents())
				if err != nil {
					uplFile.Truncate(0)
					uplFile.Close()
					os.Remove(fileName)
					return err
				}
			}
			if updloadFileReq.SchemaFile.GetHash() != nil {
				var hash hash.Hash
				switch updloadFileReq.SchemaFile.GetHash().GetMethod() {
				case schemapb.Hash_UNSPECIFIED:
					uplFile.Truncate(0)
					uplFile.Close()
					os.Remove(fileName)
					return status.Errorf(codes.InvalidArgument, "hash method unspecified")
				case schemapb.Hash_MD5:
					hash = md5.New()
				case schemapb.Hash_SHA256:
					hash = sha256.New()
				case schemapb.Hash_SHA512:
					hash = sha512.New()
				}
				rb := make([]byte, 1024*1024)
				for {
					n, err := uplFile.Read(rb)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						uplFile.Close()
						err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
						if err2 != nil {
							log.Errorf("failed to delete %s: %v", dirname, err2)
						}
						return err
					}
					_, err = hash.Write(rb[:n])
					if err != nil {
						uplFile.Close()
						err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
						if err2 != nil {
							log.Errorf("failed to delete %s: %v", dirname, err2)
						}
						return err
					}
					rb = make([]byte, 1024*1024)
				}
				calcHash := hash.Sum(nil)
				if !bytes.Equal(calcHash, updloadFileReq.SchemaFile.GetHash().GetHash()) {
					uplFile.Close()
					err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
					if err2 != nil {
						log.Errorf("failed to delete %s: %v", dirname, err2)
					}
					return status.Errorf(codes.FailedPrecondition, "file %s has wrong hash", updloadFileReq.SchemaFile.GetFileName())
				}
				uplFile.Close()
				switch updloadFileReq.SchemaFile.GetFileType() {
				case schemapb.UploadSchemaFile_MODULE:
					files = append(files, fileName)
				case schemapb.UploadSchemaFile_DEPENDENCY:
					dirs = append(dirs, fileName)
				}
				delete(handledFiles, fileName)
			}
		case *schemapb.UploadSchemaRequest_Finalize:
			if len(handledFiles) != 0 {
				err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
				if err2 != nil {
					log.Errorf("failed to delete %s: %v", dirname, err2)
				}
				return status.Errorf(codes.FailedPrecondition, "not all files are fully uploaded")
			}
			break LOOP
		default:
			err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
			if err2 != nil {
				log.Errorf("failed to delete %s: %v", dirname, err2)
			}
			return status.Errorf(codes.InvalidArgument, "unexpected message type")
		}
	}

	sc, err := schema.NewSchema(
		&config.SchemaConfig{
			Name:        name,
			Vendor:      vendor,
			Version:     version,
			Files:       files,
			Directories: dirs,
		},
	)
	if err != nil {
		err2 := os.RemoveAll(path.Join(s.config.GRPCServer.SchemaServer.SchemasDirectory, dirname))
		if err2 != nil {
			log.Errorf("failed to delete %s: %v", dirname, err2)
		}
		return err
	}
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[sc.UniqueName()] = sc
	return nil
}
