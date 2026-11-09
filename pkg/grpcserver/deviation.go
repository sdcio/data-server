package grpcserver

import (
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func (s *Server) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	var peerName string
	if ok {
		peerName = p.Addr.String()
	}
	log := logf.FromContext(ctx).WithName("WatchDeviations").WithValues("peer", peerName)
	ctx = logf.IntoContext(ctx, log)
	log.V(logf.VDebug).Info("received request", "raw-request", utils.ProtoJSON(req))

	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	}
	names := req.GetName()
	if len(names) == 0 {
		return status.Errorf(codes.InvalidArgument, "missing datastore name")
	}
	if len(names) > 1 {
		return status.Errorf(codes.InvalidArgument, "only single datastore name allowed per request")
	}

	ch, err := s.core.WatchDeviations(ctx, names[0])
	if err != nil {
		log.Error(err, "failed to watch deviations")
		if mapped := statusNotFound(err); mapped != err {
			return mapped
		}
		return status.Errorf(codes.NotFound, "unknown datastore")
	}

	log = log.WithValues("datastore-name", names[0])
	ctx = logf.IntoContext(ctx, log)

	for {
		select {
		case <-ctx.Done():
			log.Error(ctx.Err(), "stream context done", "severity", "WARN")
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(deviationEventToSdcpb(ev)); err != nil {
				return err
			}
		}
	}
}
