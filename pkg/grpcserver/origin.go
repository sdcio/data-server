package grpcserver

import (
	"context"

	"github.com/sdcio/data-server/pkg/api"
	"google.golang.org/grpc/peer"
)

// originFromContext returns the peer address as the request origin, or "" when
// the context carries no peer.
func originFromContext(ctx context.Context) api.Origin {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	return api.Origin(p.Addr.String())
}
