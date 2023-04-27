package server

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// datastore
func (s *Server) GetDataStore(ctx context.Context, req *schemapb.GetDataStoreRequest) (*schemapb.GetDataStoreResponse, error) {
	log.Debugf("Received GetDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[name]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	cands := ds.Candidates()
	rsp := &schemapb.GetDataStoreResponse{
		Name:      name,
		Datastore: make([]*schemapb.DataStore, 0, len(cands)+1),
	}
	rsp.Datastore = append(rsp.Datastore,
		&schemapb.DataStore{
			Type: *schemapb.Type_MAIN.Enum().Enum(),
			Name: name,
		},
	)
	rsp.Target = &schemapb.Target{
		Type:    ds.Config().SBI.Type,
		Address: ds.Config().SBI.Address,
		// Tls: &schemapb.TLS{
		// 	Ca:         ds.Config().SBI.TLS.CA,
		// 	Cert:       ds.Config().SBI.TLS.Cert,
		// 	Key:        ds.Config().SBI.TLS.Key,
		// 	SkipVerify: ds.Config().SBI.TLS.SkipVerify,
		// },
		// Credentials: &schemapb.Credentials{},
	}
	rsp.Schema = &schemapb.Schema{
		Name:    ds.Config().Schema.Name,
		Vendor:  ds.Config().Schema.Vendor,
		Version: ds.Config().Schema.Version,
	}
	for _, cand := range cands {
		rsp.Datastore = append(rsp.Datastore,
			&schemapb.DataStore{
				Type: *schemapb.Type_CANDIDATE.Enum(),
				Name: cand,
			},
		)
	}

	return rsp, nil
}

func (s *Server) CreateDataStore(ctx context.Context, req *schemapb.CreateDataStoreRequest) (*schemapb.CreateDataStoreResponse, error) {
	log.Debugf("Received CreateDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	switch {
	// create candidate
	case req.GetSchema() == nil && req.GetDatastore() != nil:
		s.md.RLock()
		defer s.md.RUnlock()
		ds, ok := s.datastores[name]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
		}
		switch req.GetDatastore().GetType() {
		case schemapb.Type_CANDIDATE:
			err := ds.CreateCandidate(req.GetDatastore().GetName())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "%v", err)
			}
			return &schemapb.CreateDataStoreResponse{}, nil
		default:
			return nil, status.Errorf(codes.InvalidArgument, "schema required for MAIN datastore creation")
		}
		// create main
	case req.GetSchema() != nil:
		s.md.RLock()
		_, ok := s.datastores[name]
		s.md.RUnlock()
		if ok {
			return nil, status.Errorf(codes.InvalidArgument, "datastore %s already exists", name)
		}
		s.md.Lock()
		defer s.md.Unlock()
		s.datastores[req.GetName()] = datastore.New(
			&config.DatastoreConfig{
				Name: name,
				Schema: &config.SchemaConfig{
					Name:    req.GetSchema().GetName(),
					Vendor:  req.GetSchema().GetVendor(),
					Version: req.GetSchema().GetVersion(),
				},
				SBI: &config.SBI{
					Type:    req.GetTarget().GetType(),
					Address: req.GetTarget().GetAddress(),
					TLS: &config.TLS{
						CA:         req.GetTarget().GetTls().GetCa(),
						Cert:       req.GetTarget().GetTls().GetCert(),
						Key:        req.GetTarget().GetTls().GetKey(),
						SkipVerify: req.GetTarget().GetTls().GetSkipVerify(),
					},
					Credentials: &config.Creds{
						Username: req.GetTarget().GetCredentials().GetUsername(),
						Password: req.GetTarget().GetCredentials().GetPassword(),
						Token:    req.GetTarget().GetCredentials().GetToken(),
					},
				},
			}, s.remoteSchemaClient, s.gnmiOpts...)
		return &schemapb.CreateDataStoreResponse{}, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "schema or datastore must be set")
	}
}

func (s *Server) DeleteDataStore(ctx context.Context, req *schemapb.DeleteDataStoreRequest) (*schemapb.DeleteDataStoreResponse, error) {
	log.Debugf("Received DeleteDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	s.md.RLock()
	ds, ok := s.datastores[name]
	s.md.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	switch {
	case req.GetDatastore() == nil:
		s.md.Lock()
		defer s.md.Unlock()
		ds.Stop()
		delete(s.datastores, name)
		log.Infof("deleted main %s", name)
		return &schemapb.DeleteDataStoreResponse{}, nil
	default:
		switch req.GetDatastore().GetType() {
		case *schemapb.Type_CANDIDATE.Enum():
			ds.DeleteCandidate(req.GetDatastore().GetName())
			log.Infof("datastore %s deleted candidate %s", name, req.GetDatastore().GetName())
		case *schemapb.Type_MAIN.Enum():
			s.md.Lock()
			defer s.md.Unlock()
			ds.Stop()
			delete(s.datastores, name)
			log.Infof("deleted main %s", name)
		}
		return &schemapb.DeleteDataStoreResponse{}, nil
	}
}

func (s *Server) Commit(ctx context.Context, req *schemapb.CommitRequest) (*schemapb.CommitResponse, error) {
	log.Debugf("Received CommitDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	s.md.RLock()
	ds, ok := s.datastores[name]
	s.md.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	err := ds.Commit(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &schemapb.CommitResponse{}, nil
}

func (s *Server) Rebase(ctx context.Context, req *schemapb.RebaseRequest) (*schemapb.RebaseResponse, error) {
	log.Debugf("Received RebaseDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	s.md.RLock()
	ds, ok := s.datastores[name]
	s.md.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	err := ds.Rebase(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &schemapb.RebaseResponse{}, nil
}

func (s *Server) Discard(ctx context.Context, req *schemapb.DiscardRequest) (*schemapb.DiscardResponse, error) {
	log.Debugf("Received DiscardDataStoreRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "missing name attribute")
	}
	s.md.RLock()
	ds, ok := s.datastores[name]
	s.md.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	err := ds.Discard(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &schemapb.DiscardResponse{}, nil
}
