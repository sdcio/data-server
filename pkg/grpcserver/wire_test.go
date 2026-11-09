package grpcserver

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/core"
	schemaConfig "github.com/sdcio/schema-server/pkg/config"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const ifPath = "/interface[name=ethernet-1/1]"

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.New("")
	if err != nil {
		t.Fatal(err)
	}
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
	return cfg
}

type wire struct {
	core   *core.Core
	srv    *Server
	data   sdcpb.DataServerClient
	schema sdcpb.SchemaServerClient
	runErr chan error
}

// startWire serves a Server over an in-memory listener. With start false the
// core is never started, so the server stays not ready.
func startWire(t *testing.T, cfg *config.Config, start bool) *wire {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c, srv, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1 << 20)
	srv.listen = func() (net.Listener, error) { return lis, nil }

	w := &wire{core: c, srv: srv, runErr: make(chan error, 1)}
	go func() { w.runErr <- srv.Run(ctx) }()
	if start {
		go func() { _ = c.Start(ctx) }()
	}
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = conn.Close()
		srv.Stop()
		_ = c.Stop(context.Background())
	})
	w.data = sdcpb.NewDataServerClient(conn)
	w.schema = sdcpb.NewSchemaServerClient(conn)
	return w
}

func readyWire(t *testing.T) *wire {
	t.Helper()
	w := startWire(t, testConfig(t), true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		_, err := w.data.GetDataStore(ctx, &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
		if st, _ := status.FromError(err); st.Code() != codes.Unavailable {
			return w
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server never became ready")
	return nil
}

func wantStatus(t *testing.T, err error, code codes.Code, msg string) {
	t.Helper()
	st, _ := status.FromError(err)
	if err == nil || st.Code() != code || (msg != "" && st.Message() != msg) {
		t.Fatalf("got code=%v message=%q err=%v, want code=%v message=%q", st.Code(), st.Message(), err, code, msg)
	}
}

func jsonIntent(name, descr string) *sdcpb.TransactionIntent {
	return &sdcpb.TransactionIntent{
		Intent:   name,
		Priority: 10,
		Update: []*sdcpb.Update{{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface", Key: map[string]string{"name": "ethernet-1/1"}}}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"description":"` + descr + `"}`)}},
		}},
	}
}

func TestNotReadyGate(t *testing.T) {
	w := startWire(t, testConfig(t), false)
	_, err := w.data.GetDataStore(context.Background(), &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
	wantStatus(t, err, codes.Unavailable, "not ready")
}

func TestGetDataStoreParity(t *testing.T) {
	w := readyWire(t)
	rsp, err := w.data.GetDataStore(context.Background(), &sdcpb.GetDataStoreRequest{DatastoreName: "ds1"})
	if err != nil {
		t.Fatal(err)
	}
	if rsp.GetTarget().GetType() != "noop" || rsp.GetTarget().GetPort() != 0 {
		t.Fatalf("target = %v", rsp.GetTarget())
	}
	if rsp.GetSchema().GetVendor() != "v" || rsp.GetSchema().GetVersion() != "1" {
		t.Fatalf("schema = %v", rsp.GetSchema())
	}
	if rsp.GetTarget().GetStatus() != sdcpb.TargetStatus_CONNECTED {
		t.Fatalf("status = %v", rsp.GetTarget().GetStatus())
	}

	_, err = w.data.GetDataStore(context.Background(), &sdcpb.GetDataStoreRequest{})
	wantStatus(t, err, codes.InvalidArgument, "missing datastore name attribute")
	// unknown datastores stay untyped here, as before
	_, err = w.data.GetDataStore(context.Background(), &sdcpb.GetDataStoreRequest{DatastoreName: "nope"})
	wantStatus(t, err, codes.Unknown, "unknown datastore nope")
	_, err = w.data.DeleteDataStore(context.Background(), &sdcpb.DeleteDataStoreRequest{Name: "nope"})
	wantStatus(t, err, codes.Unknown, "unknown datastore nope")
}

func TestCreateDataStoreParity(t *testing.T) {
	w := readyWire(t)
	ctx := context.Background()
	good := &sdcpb.Target{Type: "noop"}
	schema := &sdcpb.Schema{Vendor: "v", Version: "1"}

	_, err := w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{Target: good, Schema: schema})
	wantStatus(t, err, codes.InvalidArgument, "missing datastore name attribute")

	// the existing-name check precedes target validation
	_, err = w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{DatastoreName: "ds1", Target: &sdcpb.Target{Type: "pigeon"}, Schema: schema})
	wantStatus(t, err, codes.InvalidArgument, "datastore ds1 already exists")

	_, err = w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{DatastoreName: "new", Target: &sdcpb.Target{Type: "pigeon"}, Schema: schema})
	wantStatus(t, err, codes.InvalidArgument, `unknown targetconnection protocol type "pigeon"`)

	_, err = w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{DatastoreName: "new", Schema: schema,
		Target: &sdcpb.Target{Type: "netconf", ProtocolOptions: &sdcpb.Target_NetconfOpts{NetconfOpts: &sdcpb.NetconfOptions{CommitCandidate: 99}}}})
	wantStatus(t, err, codes.Unknown, "unknown commitDatastore: 99")

	_, err = w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{DatastoreName: "new", Target: good, Schema: schema,
		Sync: &sdcpb.Sync{Config: []*sdcpb.SyncConfig{{Target: &sdcpb.Target{Type: "pigeon"}}}}})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument || !strings.HasPrefix(st.Message(), "unknown targetsync protocol") {
		t.Fatalf("got %v", err)
	}

	if _, err = w.data.CreateDataStore(ctx, &sdcpb.CreateDataStoreRequest{DatastoreName: "new", Target: good, Schema: schema}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.data.DeleteDataStore(ctx, &sdcpb.DeleteDataStoreRequest{Name: "new"}); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionWire(t *testing.T) {
	w := readyWire(t)
	ctx := context.Background()

	set, err := w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{
		DatastoreName: "ds1", TransactionId: "t1", Intents: []*sdcpb.TransactionIntent{jsonIntent("i1", "one")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.GetUpdate()) == 0 {
		t.Fatalf("no updates in %v", set)
	}
	if _, err = w.data.TransactionConfirm(ctx, &sdcpb.TransactionConfirmRequest{DatastoreName: "ds1", TransactionId: "t1"}); err != nil {
		t.Fatal(err)
	}
	got, err := w.data.GetIntent(ctx, &sdcpb.GetIntentRequest{DatastoreName: "ds1", Intent: "i1", Format: sdcpb.Format_Intent_Format_JSON})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPriority() != 10 || !strings.Contains(string(got.GetBlob()), "one") {
		t.Fatalf("intent = %v", got)
	}

	if _, err = w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{
		DatastoreName: "ds1", TransactionId: "t2", Intents: []*sdcpb.TransactionIntent{jsonIntent("i1", "two")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.data.TransactionCancel(ctx, &sdcpb.TransactionCancelRequest{DatastoreName: "ds1", TransactionId: "t2"}); err != nil {
		t.Fatal(err)
	}
	got, err = w.data.GetIntent(ctx, &sdcpb.GetIntentRequest{DatastoreName: "ds1", Intent: "i1"})
	if err != nil || !strings.Contains(string(got.GetBlob()), "one") {
		t.Fatalf("cancel did not roll back: %v %v", got, err)
	}

	if _, err = w.data.BlameConfig(ctx, &sdcpb.BlameConfigRequest{DatastoreName: "ds1"}); err != nil {
		t.Fatal(err)
	}
	li, err := w.data.ListIntent(ctx, &sdcpb.ListIntentRequest{DatastoreName: "ds1"})
	if err != nil || len(li.GetIntent()) == 0 {
		t.Fatalf("list intents: %v %v", li, err)
	}
}

func TestTransactionErrorsWire(t *testing.T) {
	w := readyWire(t)
	ctx := context.Background()

	_, err := w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{TransactionId: "x"})
	wantStatus(t, err, codes.InvalidArgument, "missing datastore name")
	_, err = w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{DatastoreName: "nope", TransactionId: "x"})
	wantStatus(t, err, codes.NotFound, "unknown datastore nope")
	_, err = w.data.TransactionConfirm(ctx, &sdcpb.TransactionConfirmRequest{DatastoreName: "nope"})
	wantStatus(t, err, codes.NotFound, "unknown datastore nope")
	_, err = w.data.TransactionCancel(ctx, &sdcpb.TransactionCancelRequest{})
	wantStatus(t, err, codes.InvalidArgument, "missing datastore name")

	high := jsonIntent("p", "x")
	high.Priority = 1<<31 - 1
	_, err = w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{DatastoreName: "ds1", TransactionId: "p", Intents: []*sdcpb.TransactionIntent{high}})
	if st, _ := status.FromError(err); st.Code() != codes.Unknown || !strings.HasPrefix(st.Message(), "user settable priority not allowed above") {
		t.Fatalf("priority: %v", err)
	}

	str := jsonIntent("s", "x")
	str.Update[0].Value = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "x"}}
	_, err = w.data.TransactionSet(ctx, &sdcpb.TransactionSetRequest{DatastoreName: "ds1", TransactionId: "s", Intents: []*sdcpb.TransactionIntent{str}})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("non-json value must be rejected, got %v", err)
	}
}

func TestGetIntentErrorsWire(t *testing.T) {
	w := readyWire(t)
	ctx := context.Background()
	_, err := w.data.GetIntent(ctx, &sdcpb.GetIntentRequest{Intent: "i"})
	wantStatus(t, err, codes.InvalidArgument, "missing datastore name")
	_, err = w.data.GetIntent(ctx, &sdcpb.GetIntentRequest{DatastoreName: "ds1"})
	wantStatus(t, err, codes.InvalidArgument, "missing intent name")
	_, err = w.data.GetIntent(ctx, &sdcpb.GetIntentRequest{DatastoreName: "nope", Intent: "i"})
	wantStatus(t, err, codes.NotFound, "unknown datastore nope")
	_, err = w.data.ListIntent(ctx, &sdcpb.ListIntentRequest{DatastoreName: "nope"})
	wantStatus(t, err, codes.NotFound, "unknown datastore nope")
}

func TestWatchDeviationsWire(t *testing.T) {
	w := readyWire(t)

	for name, names := range map[string][]string{"none": nil, "many": {"a", "b"}} {
		t.Run(name, func(t *testing.T) {
			s, err := w.data.WatchDeviations(context.Background(), &sdcpb.WatchDeviationRequest{Name: names})
			if err == nil {
				_, err = s.Recv()
			}
			wantStatus(t, err, codes.InvalidArgument, "")
		})
	}

	s, err := w.data.WatchDeviations(context.Background(), &sdcpb.WatchDeviationRequest{Name: []string{"nope"}})
	if err == nil {
		_, err = s.Recv()
	}
	wantStatus(t, err, codes.NotFound, "unknown datastore")

	ctx, cancel := context.WithCancel(context.Background())
	s, err = w.data.WatchDeviations(ctx, &sdcpb.WatchDeviationRequest{Name: []string{"ds1"}})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err = s.Recv(); status.Code(err) != codes.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestSchemaWire(t *testing.T) {
	w := readyWire(t)
	rsp, err := w.schema.GetSchemaDetails(context.Background(), &sdcpb.GetSchemaDetailsRequest{Schema: &sdcpb.Schema{Vendor: "v", Version: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if rsp.GetSchema().GetVendor() != "v" || len(rsp.GetFile()) == 0 {
		t.Fatalf("details = %v", rsp)
	}
	if _, err = w.schema.ListSchema(context.Background(), &sdcpb.ListSchemaRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestStreamsNotReadyAreUnavailable(t *testing.T) {
	w := startWire(t, testConfig(t), false)
	s, err := w.schema.GetSchemaElements(context.Background(), &sdcpb.GetSchemaRequest{Schema: &sdcpb.Schema{Vendor: "v", Version: "1"}})
	if err == nil {
		_, err = s.Recv()
	}
	wantStatus(t, err, codes.Unavailable, "not ready")
}

func TestRunReturnsCoreStartFailure(t *testing.T) {
	cfg := testConfig(t)
	cfg.Datastores[0].Schema.Name = "missing"
	w := startWire(t, cfg, true)
	select {
	case err := <-w.runErr:
		if err == nil {
			t.Fatal("Run must report the failed start")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestRunReturnsNilWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c, srv, err := New(ctx, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	lis := bufconn.Listen(1 << 20)
	srv.listen = func() (net.Listener, error) { return lis, nil }
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunRetriesListenFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, srv, err := New(ctx, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(context.Background()) })
	lis := bufconn.Listen(1 << 20)
	attempts := 0
	srv.listen = func() (net.Listener, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("address in use")
		}
		return lis, nil
	}
	go func() { _ = srv.Run(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := lis.Dial()
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("listener never served after %d attempts", attempts)
}

func TestBlameTreeOmitsEmptyValues(t *testing.T) {
	e := blameTree(&api.BlameNode{Name: "n", Value: "v", Children: []*api.BlameNode{{Name: "c"}}})
	if e.GetValue().GetStringVal() != "v" || e.GetDeviationValue() != nil {
		t.Fatalf("root = %v", e)
	}
	if len(e.GetChilds()) != 1 || e.GetChilds()[0].GetValue() != nil {
		t.Fatalf("child = %v", e.GetChilds())
	}
	if blameTree(nil) != nil {
		t.Fatal("nil node must stay nil")
	}
}
