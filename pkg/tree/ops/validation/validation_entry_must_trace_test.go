package validation_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/data-server/pkg/tree"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/ops/validation"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
)

type capturedEval struct {
	meta    validation.MustTraceMeta
	events  []xpath.TraceEvent
	outcome validation.MustTraceOutcome
	ended   bool
}

type recordingMustTracer struct {
	mu     sync.Mutex
	filter func(validation.MustTraceMeta) bool
	evals  []*capturedEval
}

func (r *recordingMustTracer) BeginMustTrace(meta validation.MustTraceMeta) validation.MustEvalTracer {
	if r.filter != nil && !r.filter(meta) {
		return nil
	}
	ce := &capturedEval{meta: meta}
	r.mu.Lock()
	r.evals = append(r.evals, ce)
	r.mu.Unlock()
	return ce
}

func (c *capturedEval) TraceEvent(ev xpath.TraceEvent) {
	c.events = append(c.events, ev)
}

func (c *capturedEval) End(outcome validation.MustTraceOutcome) {
	c.outcome = outcome
	c.ended = true
}

func buildMustTestTree(t *testing.T, yangModule string, jsonConfig string) *tree.RootEntry {
	t.Helper()
	ctx := context.Background()
	yangDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(yangDir, "must-test.yang"), []byte(yangModule), 0o600); err != nil {
		t.Fatalf("write yang: %v", err)
	}

	st := memstore.New()
	t.Cleanup(func() { _ = st.Close() })
	sc := schema.NewLocalClient(st)
	scRef := &sdcpb.Schema{
		Name:    "must-test",
		Vendor:  "test",
		Version: "1.0.0",
	}
	if _, err := sc.CreateSchema(ctx, &sdcpb.CreateSchemaRequest{
		Schema: scRef,
		File:   []string{yangDir},
	}); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	scCfg := &config.SchemaConfig{
		Name:    scRef.Name,
		Vendor:  scRef.Vendor,
		Version: scRef.Version,
	}
	scb := schemaClient.NewSchemaClientBound(scCfg, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	tc := tree.NewTreeContext(scb, sharedPool)
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}

	var raw any
	if err := json.Unmarshal([]byte(jsonConfig), &raw); err != nil {
		t.Fatalf("unmarshal json config: %v", err)
	}
	flags := types.NewUpdateInsertFlags()
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(raw, "intent-a", 100, false), flags, sharedPool); err != nil {
		t.Fatalf("ImportConfig: %v", err)
	}
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("FinishInsertionPhase: %v", err)
	}
	return root
}

const mustTestYang = `module must-test {
  yang-version 1.1;
  namespace "urn:sdcio:must-test";
  prefix "mt";

  container network {
    leaf max-mtu {
      type uint32;
    }
    leaf optional-tag {
      type string;
    }
    list interface {
      key "name";
      leaf name {
        type string;
      }
      leaf mtu {
        type uint32;
        must ". <= ../../max-mtu and not(../../optional-tag = 'blocked')" {
          error-message "interface mtu exceeds network max-mtu or blocked";
        }
      }
      leaf description {
        type string;
        must "string-length(.) > 0" {
          error-message "description must not be empty";
        }
      }
    }
  }
}
`

func TestValidateMustTracer_CapturesProgramAdapterIOAndSwallowedNavigateError(t *testing.T) {
	ctx := context.Background()
	root := buildMustTestTree(t, mustTestYang, `{
		"network": {
			"max-mtu": 1500,
			"interface": [
				{
					"name": "eth0",
					"mtu": 9000,
					"description": "uplink"
				}
			]
		}
	}`)

	tracer := &recordingMustTracer{
		filter: func(m validation.MustTraceMeta) bool {
			return strings.Contains(m.KeylessPath, "/network/interface/mtu")
		},
	}

	vCfg := config.NewValidationConfig()
	vCfg.DisableConcurrency = true
	res, _ := validation.Validate(
		ctx,
		root.Entry,
		vCfg,
		pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		validation.WithMustTracer(tracer),
	)

	if len(res.ErrorsStr()) != 1 {
		t.Fatalf("expected 1 validation error, got %v", res.ErrorsStr())
	}

	if len(tracer.evals) != 1 {
		t.Fatalf("expected filter to capture 1 must evaluation, got %d", len(tracer.evals))
	}
	ev := tracer.evals[0]
	if !ev.ended {
		t.Fatalf("expected End() to be called on MustEvalTracer")
	}
	if ev.meta.Path != "/network/interface[name=eth0]/mtu" || ev.meta.KeylessPath != "/network/interface/mtu" {
		t.Fatalf("unexpected meta paths: %+v", ev.meta)
	}
	if ev.meta.Owner != "intent-a" || ev.meta.Value != "9000" {
		t.Fatalf("expected owner=intent-a value=9000, got %+v", ev.meta)
	}
	if ev.meta.ErrorMessage != "interface mtu exceeds network max-mtu or blocked" {
		t.Fatalf("unexpected error-message in meta: %+v", ev.meta)
	}
	if ev.outcome.Status != validation.MustTraceStatusFailed || ev.outcome.Result == nil || *ev.outcome.Result != false {
		t.Fatalf("unexpected outcome: %+v", ev.outcome)
	}

	var sawProgram, sawMaxMtuValue, sawOptionalTagFallback, sawVMInstr bool
	for _, e := range ev.events {
		switch e.Kind {
		case validation.TraceEventMustProgram:
			if listing, _ := e.Details["program"].(string); strings.Contains(listing, "--- machine start ---") {
				sawProgram = true
			}
		case validation.TraceEventAdapterGetValue:
			if e.Node == "/network/max-mtu" && e.Result == "NUMBER(1500)" && e.Details["owner"] == "intent-a" {
				sawMaxMtuValue = true
			}
		case validation.TraceEventAdapterNavigate:
			if e.Details["fallback"] == "empty_nodeset" && e.Error != "" && strings.Contains(e.Details["target"].(string), "optional-tag") {
				sawOptionalTagFallback = true
			}
		case xpath.TraceEventVMInstr:
			sawVMInstr = true
		}
	}
	if !sawProgram || !sawMaxMtuValue || !sawOptionalTagFallback || !sawVMInstr {
		t.Fatalf("missing expected trace events: program=%v maxMtu=%v optionalFallback=%v vmInstr=%v; events=%+v",
			sawProgram, sawMaxMtuValue, sawOptionalTagFallback, sawVMInstr, ev.events)
	}
}

func TestValidateMustTracer_EmitsSkippedAndStackUnderflowOutcomes(t *testing.T) {
	ctx := context.Background()
	underflowYang := `module must-underflow {
  yang-version 1.1;
  namespace "urn:sdcio:must-underflow";
  prefix "mu";

  container box {
    leaf a {
      type string;
      must "../box[. = 'hello']" {
        error-message "predicate dot triggers stack underflow";
      }
    }
    leaf deleted-leaf {
      type string;
      must "string-length(.) > 3" {
        error-message "deleted leaf check";
      }
    }
  }
}
`
	yangDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(yangDir, "must-underflow.yang"), []byte(underflowYang), 0o600); err != nil {
		t.Fatalf("write yang: %v", err)
	}
	st := memstore.New()
	t.Cleanup(func() { _ = st.Close() })
	sc := schema.NewLocalClient(st)
	scRef := &sdcpb.Schema{Name: "must-underflow", Vendor: "test", Version: "1.0.0"}
	if _, err := sc.CreateSchema(ctx, &sdcpb.CreateSchemaRequest{Schema: scRef, File: []string{yangDir}}); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}
	scb := schemaClient.NewSchemaClientBound(&config.SchemaConfig{Name: scRef.Name, Vendor: scRef.Vendor, Version: scRef.Version}, sc)
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(scb, sharedPool))
	if err != nil {
		t.Fatalf("NewTreeRoot: %v", err)
	}

	var raw any
	if err := json.Unmarshal([]byte(`{"box":{"a":"hello","deleted-leaf":"hi"}}`), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := root.ImportConfig(ctx, &sdcpb.Path{}, jsonImporter.NewJsonTreeImporter(raw, "intent-a", 100, false), types.NewUpdateInsertFlags(), sharedPool); err != nil {
		t.Fatalf("ImportConfig: %v", err)
	}

	// Mark /box/deleted-leaf for deletion prior to FinishInsertionPhase so RemainsToExist() evaluates false.
	delPath := &sdcpb.Path{
		IsRootBased: true,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("box", nil),
			sdcpb.NewPathElem("deleted-leaf", nil),
		},
	}
	delEntry, err := ops.NavigateSdcpbPath(ctx, root.Entry, delPath)
	if err != nil {
		t.Fatalf("navigate deleted-leaf: %v", err)
	}
	delEntry.GetLeafVariants().MarkOwnerForDeletion("intent-a", false)
	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatalf("FinishInsertionPhase: %v", err)
	}

	tracer := &recordingMustTracer{}
	vCfg := config.NewValidationConfig()
	vCfg.DisableConcurrency = true
	res, _ := validation.Validate(
		ctx,
		root.Entry,
		vCfg,
		pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		validation.WithMustTracer(tracer),
	)
	if len(res.ErrorsStr()) != 0 {
		t.Fatalf("expected stack underflow and deleted leaf to produce 0 validation errors, got %v", res.ErrorsStr())
	}

	statuses := map[string]validation.MustTraceOutcome{}
	for _, ev := range tracer.evals {
		statuses[ev.meta.Path] = ev.outcome
	}
	if got, ok := statuses["/box/a"]; !ok || got.Status != validation.MustTraceStatusStackUnderflow {
		t.Fatalf("expected /box/a status stack_underflow_ignored, got %+v (all=%+v)", got, statuses)
	}
	if got, ok := statuses["/box/deleted-leaf"]; !ok || got.Status != validation.MustTraceStatusSkipped || got.SkipReason == "" {
		t.Fatalf("expected /box/deleted-leaf status skipped with reason, got %+v (all=%+v)", got, statuses)
	}
}

func TestValidateMustTracer_CapturesListCountAndFollowLeafRef(t *testing.T) {
	ctx := context.Background()
	yangMod := `module must-deref-count {
  yang-version 1.1;
  namespace "urn:sdcio:must-deref-count";
  prefix "mdc";

  container net {
    list iface {
      key "name";
      leaf name {
        type string;
      }
      leaf mtu {
        type uint32;
      }
    }
    leaf primary-iface {
      type leafref {
        path "../iface/name";
      }
      must "count(../iface) >= 2 and deref(.)/../mtu >= 1500" {
        error-message "need at least 2 ifaces and primary mtu >= 1500";
      }
    }
  }
}
`
	root := buildMustTestTree(t, yangMod, `{
		"net": {
			"iface": [
				{"name": "eth0", "mtu": 9000},
				{"name": "eth1", "mtu": 1500}
			],
			"primary-iface": "eth0"
		}
	}`)

	tracer := &recordingMustTracer{}
	vCfg := config.NewValidationConfig()
	vCfg.DisableConcurrency = true
	res, _ := validation.Validate(
		ctx,
		root.Entry,
		vCfg,
		pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)),
		validation.WithMustTracer(tracer),
	)
	if len(res.ErrorsStr()) != 0 {
		t.Fatalf("expected 0 validation errors, got %v", res.ErrorsStr())
	}
	if len(tracer.evals) != 1 {
		t.Fatalf("expected 1 traced must evaluation, got %d", len(tracer.evals))
	}
	ev := tracer.evals[0]
	if ev.outcome.Status != validation.MustTraceStatusPassed || ev.outcome.Result == nil || !*ev.outcome.Result {
		t.Fatalf("expected passed outcome, got %+v", ev.outcome)
	}

	var sawListNodeset, sawLeafRef bool
	for _, e := range ev.events {
		switch e.Kind {
		case validation.TraceEventAdapterGetValue:
			if e.Node == "/net/iface" && e.Details["entry_kind"] == "list" && e.Result == "NODESET(len=2, placeholder_nodes=2)" {
				sawListNodeset = true
			}
		case validation.TraceEventAdapterFollowLeafRef:
			if e.Details["resolved_path"] == "/net/iface[name=eth0]/name" {
				sawLeafRef = true
			}
		}
	}
	if !sawListNodeset || !sawLeafRef {
		t.Fatalf("expected list get_value (%v) and follow_leafref (%v) events; got %+v", sawListNodeset, sawLeafRef, ev.events)
	}
}

