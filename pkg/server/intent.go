package server

import (
	"context"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func (s *Server) GetIntent(ctx context.Context, req *sdcpb.GetIntentRequest) (*sdcpb.GetIntentResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received GetIntent request %v from peer %s", req, pr.Addr.String())

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	if req.GetIntent() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing intent name")
	}
	if req.GetPriority() == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing intent priority")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[req.GetName()]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", req.GetName())
	}
	return ds.GetIntent(ctx, req)
}

func (s *Server) SetIntent(ctx context.Context, req *sdcpb.SetIntentRequest) (*sdcpb.SetIntentResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received GetIntent request %v from peer %s", req, pr.Addr.String())

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	if req.GetIntent() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing intent name")
	}
	if len(req.GetUpdate()) == 0 && !req.GetDelete() {
		return nil, status.Error(codes.InvalidArgument, "updates or a delete flag must be set")
	}
	if len(req.GetUpdate()) != 0 && req.GetDelete() {
		return nil, status.Error(codes.InvalidArgument, "both updates and the delete flag cannot be set at the same time")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[req.GetName()]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", req.GetName())
	}
	return ds.SetIntent(ctx, req)
}

func (s *Server) ListIntent(ctx context.Context, req *sdcpb.ListIntentRequest) (*sdcpb.ListIntentResponse, error) {
	pr, _ := peer.FromContext(ctx)
	log.Debugf("received ListIntent request %v from peer %s", req, pr.Addr.String())

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing datastore name")
	}
	s.md.RLock()
	defer s.md.RUnlock()
	ds, ok := s.datastores[req.GetName()]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %s", req.GetName())
	}
	return ds.ListIntent(ctx, req)
}
