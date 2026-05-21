package validation_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/openconfig/ygot/ygot"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	xmlimporter "github.com/sdcio/data-server/pkg/tree/importer/xml"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	importDeviceOwner    = "owner1"
	importDevicePriority = int32(5)
)

// importDeviceJSON creates a fresh tree, imports req via JSON (owner "owner1", priority 5),
// finishes the insertion phase, and returns the root entry and a shared pool ready for validation.
func importDeviceJSON(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()
	return importDeviceJSONWithOwnerPrio(t, ctx, scb, importDeviceOwner, importDevicePriority, req)
}

// importDeviceJSONWithOwnerPrio is like importDeviceJSON but uses the given intent owner and priority.
func importDeviceJSONWithOwnerPrio(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, owner string, prio int32, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()
	return importDevice(t, ctx, scb, func(t *testing.T, root *tree.RootEntry) {
		t.Helper()
		if _, err := testhelper.LoadYgotStructIntoTreeRoot(ctx, req, root.Entry, owner, prio, false, treetypes.NewUpdateInsertFlags()); err != nil {
			t.Fatal(err)
		}
	})
}

// importDeviceIntentJSON is like importDeviceJSON but applies the same device as RFC7951 JSON
// through ExpandAndConvertIntent (northbound-style expansion), for parity tests against structured import.
func importDeviceIntentJSON(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()
	return importDeviceExpandJSONWithOwnerPrio(t, ctx, scb, importDeviceOwner, importDevicePriority, req)
}

// importDeviceExpandJSONWithOwnerPrio applies RFC7951 JSON for req through ExpandAndConvertIntent
// with the given owner and priority (same entry point as gNMI Sync uses for "running").
func importDeviceExpandJSONWithOwnerPrio(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, owner string, prio int32, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()
	return importDevice(t, ctx, scb, func(t *testing.T, root *tree.RootEntry) {
		t.Helper()
		jstr, err := ygot.EmitJSON(req, &ygot.EmitJSONConfig{
			Format:         ygot.RFC7951,
			SkipValidation: true,
		})
		if err != nil {
			t.Fatalf("EmitJSON: %v", err)
		}
		expanded, err := treetypes.ExpandAndConvertIntent(ctx, scb, owner, prio, []*sdcpb.Update{{
			Path: &sdcpb.Path{},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(jstr)},
			},
		}}, time.Now().Unix())
		if err != nil {
			t.Fatalf("ExpandAndConvertIntent: %v", err)
		}
		flags := treetypes.NewUpdateInsertFlags()
		for _, pu := range expanded {
			if _, err := ops.AddUpdateRecursive(ctx, root.Entry, pu.GetPath(), pu.GetUpdate(), flags); err != nil {
				t.Fatalf("AddUpdateRecursive: %v", err)
			}
		}
	})
}

func importDevice(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, apply func(t *testing.T, root *tree.RootEntry)) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	apply(t, root)

	if err := root.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}

	return root, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
}

// importDeviceJSONThenCanonicalXMLReimport loads req via JSON (same owner/priority as importDeviceJSON),
// exports the tree with ops.ToXML, then imports that document with the XML tree importer into a fresh
// tree. Used to assert NETCONF/XML-style ingress sees the same validation outcomes as the JSON path
// when the logical configuration is equivalent (PRD 006).
func importDeviceJSONThenCanonicalXMLReimport(t *testing.T, ctx context.Context, scb schemaClient.SchemaClientBound, req *sdcio_schema.Device) (*tree.RootEntry, *pool.SharedTaskPool) {
	t.Helper()
	root, _ := importDeviceJSON(t, ctx, scb, req)
	xmlDoc, err := ops.ToXML(ctx, root.Entry, false, false, false, false)
	if err != nil {
		t.Fatalf("ToXML: %v", err)
	}
	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	rootXML, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	sharedPool := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
	if _, err := rootXML.ImportConfig(ctx, nil, xmlimporter.NewXmlTreeImporter(&xmlDoc.Element, importDeviceOwner, importDevicePriority, false), treetypes.NewUpdateInsertFlags(), sharedPool); err != nil {
		t.Fatalf("ImportConfig XML: %v", err)
	}
	if err := rootXML.FinishInsertionPhase(ctx); err != nil {
		t.Fatal(err)
	}
	return rootXML, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))
}
