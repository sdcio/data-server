package grpcserver

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// WatchDeviations streams deviation events of one datastore until the client
// goes away or the core closes the watch. A missing peer, a missing or
// multiple datastore names are InvalidArgument; any failure to open the watch
// is reported as NotFound "unknown datastore".
func (s *Server) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	ctx := stream.Context()
	var peerName string
	p, hasPeer := peer.FromContext(ctx)
	if hasPeer {
		peerName = p.Addr.String()
	}
	ctx, log := begin(ctx, "WatchDeviations", req, "peer", peerName)

	switch {
	case !hasPeer:
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	case len(req.GetName()) == 0:
		return status.Errorf(codes.InvalidArgument, "missing datastore name")
	case len(req.GetName()) > 1:
		return status.Errorf(codes.InvalidArgument, "only single datastore name allowed per request")
	}

	ch, err := s.core.WatchDeviations(ctx, req.GetName()[0])
	if err != nil {
		log.Error(err, "failed to get datastore")
		return status.Errorf(codes.NotFound, "unknown datastore")
	}
	log = log.WithValues("datastore-name", req.GetName()[0])

	for {
		select {
		case <-ctx.Done():
			log.Error(ctx.Err(), "stream context done", "severity", "WARN")
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(deviationResponse(ev)); err != nil {
				return err
			}
		}
	}
}
