package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
	wg := new(sync.WaitGroup)
	wg.Add(1)
	nCh := make(chan *schemapb.GetDataResponse)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stream.Context().Done():
				return
			case rsp, ok := <-nCh:
				if !ok {
					return
				}
				fmt.Println(rsp)
				err := stream.Send(rsp)
				if err != nil {
					if strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "EOF") {
						return
					}
					log.Errorf("GetData stream send err :%v", err)
				}
			}
		}
	}()
	err := ds.Get(stream.Context(), req, nCh)
	if err != nil {
		return err
	}
	wg.Wait()
	return nil
}

func (s *Server) SetData(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	log.Debugf("received SetDataRequest: %v", req)
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
	log.Debugf("received DiffRequest: %v", req)
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
	log.Debugf("received SubscribeRequest: %v", req)
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
