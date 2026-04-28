package ops_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func TestToXPath_FromConfig1(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, testhelper.Config1(), root.Entry, "owner1", 5, false, testhelper.FlagsNew)
	if err != nil {
		t.Fatal(err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	pvs, err := ops.ToXPath(ctx, root.Entry, false, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(pvs.GetPathValues()) == 0 {
		t.Fatal("ToXPath() returned no path values")
	}

	got := make(map[string]string, len(pvs.GetPathValues()))
	for _, pv := range pvs.GetPathValues() {
		if pv.GetPath() == nil {
			t.Fatal("ToXPath() returned a path value with nil path")
		}
		if pv.GetValue() == nil {
			t.Fatalf("ToXPath() returned nil value for path %s", pv.GetPath().ToXPath(false))
		}

		xPath := pv.GetPath().ToXPath(false)
		if strVal := pv.GetValue().GetStringVal(); strVal != "" {
			got[xPath] = strVal
			continue
		}
		got[xPath] = pv.GetValue().String()
	}

	if got["/interface[name=ethernet-1/1]/description"] != "Foo" {
		t.Fatalf("missing or mismatching interface description, got %q", got["/interface[name=ethernet-1/1]/description"])
	}

	if got["/network-instance[name=default]/description"] != "Default NI" {
		t.Fatalf("missing or mismatching network-instance description, got %q", got["/network-instance[name=default]/description"])
	}

	if got["/patterntest"] != "hallo 00" {
		t.Fatalf("missing or mismatching patterntest, got %q", got["/patterntest"])
	}
}

func TestToXPath_OnlyNewOrUpdated_WithSameNewAndExistingConfig1(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := tree.NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0)))
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	owner := "owner1"
	prio := int32(5)

	_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, testhelper.Config1(), root.Entry, owner, prio, false, testhelper.FlagsExisting)
	if err != nil {
		t.Fatal(err)
	}

	_, err = testhelper.LoadYgotStructIntoTreeRoot(ctx, testhelper.Config1(), root.Entry, owner, prio, false, testhelper.FlagsNew)
	if err != nil {
		t.Fatal(err)
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Fatal(err)
	}

	onlyNewOrUpdated, err := ops.ToXPath(ctx, root.Entry, true, false)
	if err != nil {
		t.Fatal(err)
	}

	allValues, err := ops.ToXPath(ctx, root.Entry, false, false)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(onlyNewOrUpdated.GetPathValues()), len(allValues.GetPathValues()); got != want {
		t.Fatalf("ToXPath() onlyNewOrUpdated count mismatch: got %d, want %d", got, want)
	}

	containsPatternTest := false
	for _, pv := range onlyNewOrUpdated.GetPathValues() {
		if pv.GetPath() != nil && pv.GetPath().ToXPath(false) == "/patterntest" {
			containsPatternTest = true
			break
		}
	}
	if !containsPatternTest {
		t.Fatal("ToXPath() did not include expected /patterntest path")
	}
}