package grpcserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	schemaConfig "github.com/sdcio/schema-server/pkg/config"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func startWireStack(t *testing.T) (api sdcpb.DataServerClient, cleanup func()) {
	t.Helper()
	cfg, err := config.New("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.GRPCServer.Address = "passthrough:///bufnet"
	cfg.SchemaStore = &schemaConfig.SchemaStoreConfig{
		Type: schemaConfig.StoreTypeMemory,
		Schemas: []*schemaConfig.SchemaConfig{{
			Vendor: "v", Version: "1",
			Files: []string{"../../tests/schema/sdcio_model.yang"}, Directories: []string{"../../tests/schema"},
		}},
	}
	cfg.Cache = &config.CacheConfig{Type: "local", StoreType: "badgerdbsharded", Dir: t.TempDir()}
	cfg.Datastores = []*config.DatastoreConfig{{
		Name: "ds1", Schema: &config.SchemaConfig{Vendor: "v", Version: "1"},
		SBI: &config.SBI{Type: "noop"}, Sync: &config.Sync{},
	}}
	for _, ds := range cfg.Datastores {
		if err := ds.ValidateSetDefaults(); err != nil {
			t.Fatal(err)
		}
	}

	const bufSize = 1 << 20
	lis := bufconn.Listen(bufSize)
	ctx := context.Background()

	c, srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Start(ctx) }()
	go func() {
		if err := srv.srv.Serve(lis); err != nil {
			t.Log(err)
		}
	}()
	go func() {
		if err := c.WaitReady(ctx); err != nil {
			t.Error(err)
			return
		}
		srv.ready.Store(true)
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	cleanup = func() {
		srv.Stop()
		_ = c.Stop(ctx)
		_ = conn.Close()
	}
	return sdcpb.NewDataServerClient(conn), cleanup
}

func TestWireNotReady(t *testing.T) {
	cfg, err := config.New("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.SchemaStore = &schemaConfig.SchemaStoreConfig{Type: schemaConfig.StoreTypeMemory}
	cfg.Cache = &config.CacheConfig{Type: "local", StoreType: "badgerdbsharded", Dir: t.TempDir()}
	const bufSize = 1 << 20
	lis := bufconn.Listen(bufSize)
	ctx := context.Background()
	c, srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop(); _ = c.Stop(ctx) })

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := sdcpb.NewDataServerClient(conn)
	_, err = client.GetDataStore(ctx, &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unavailable || st.Message() != "not ready" {
		t.Fatalf("got %v", err)
	}
}

func TestWireGetDatastore(t *testing.T) {
	client, cleanup := startWireStack(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		_, err := client.GetDataStore(ctx, &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unavailable {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		break
	}
}

func TestWireTransactionNotFound(t *testing.T) {
	client, cleanup := startWireStack(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	waitReady(ctx, t, client)
	_, err := client.TransactionConfirm(ctx, &sdcpb.TransactionConfirmRequest{DatastoreName: "nope", TransactionId: "x"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func waitReady(ctx context.Context, t *testing.T, client sdcpb.DataServerClient) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, err := client.GetDataStore(ctx, &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unavailable && st.Message() == "not ready" {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return
	}
	t.Fatal("timed out waiting for ready")
}
