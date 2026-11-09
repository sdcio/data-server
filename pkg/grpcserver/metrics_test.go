package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// With a registerer supplied and cfg.Prometheus unset, server metrics land on
// the caller's registry and no metrics listener is configured.
func TestWithMetricsRegisterer(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := testConfig(t)
	cfg.Prometheus = nil

	w := startWire(t, cfg, true, WithMetricsRegisterer(reg))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Calls rejected as not ready bypass the metrics interceptor; wait for one to pass.
	for ctx.Err() == nil {
		_, err := w.data.GetDataStore(ctx, &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
		if st, _ := status.FromError(err); st.Code() != codes.Unavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "grpc_server_started_total" {
			found = true
		}
	}
	if !found {
		t.Fatal("grpc_server_started_total not registered on supplied registry")
	}
}
