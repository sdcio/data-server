package server

import (
	"context"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// data
func (s *Server) GetData(req *schemapb.GetDataRequest, stream schemapb.DataServer_GetDataServer) error {
	pr, _ := peer.FromContext(stream.Context())
	log.Debugf("received GetData request %v from peer %s", req, pr.Addr.String())
	name := req.GetName()
	if name == "" {
		return status.Error(codes.InvalidArgument, "missing datastore name")
	}
	switch req.GetDataType() {
	case schemapb.DataType_STATE:
		if req.GetDatastore().GetType() == schemapb.Type_CANDIDATE {
			return status.Error(codes.InvalidArgument, "a candidate datastore does not store state data")
		}
	}
	if len(req.GetPath()) == 0 {
		return status.Error(codes.InvalidArgument, "missing path attribute")
	}

	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[name]
	if !ok {
		return status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	nCh := make(chan *schemapb.GetDataResponse)
	go func() {
		for {
			select {
			case <-stream.Context().Done():
				return
			case rsp, ok := <-nCh:
				if !ok {
					return
				}
				err := stream.Send(rsp)
				if err != nil && !strings.Contains(err.Error(), "context canceled") {
					log.Errorf("GetData stream send err :%v", err)
				}
			}
		}
	}()
	err := ds.Get(stream.Context(), req, nCh)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) SetData(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	log.Infof("received SetDataRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[name]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	return ds.Set(ctx, req)
}

func (s *Server) Diff(ctx context.Context, req *schemapb.DiffRequest) (*schemapb.DiffResponse, error) {
	log.Infof("received DiffRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[name]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	return ds.Diff(ctx, req)
}

func (s *Server) Subscribe(req *schemapb.SubscribeRequest, stream schemapb.DataServer_SubscribeServer) error {
	log.Infof("received SubscribeRequest: %v", req)
	name := req.GetName()
	if name == "" {
		return status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	s.md.RLock()
	ds, ok := s.datastores[name]
	s.md.RUnlock()
	if !ok {
		return status.Errorf(codes.InvalidArgument, "unknown datastore %s", name)
	}
	ds.Subscribe(req, stream)
	return status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}
