package datastore

import (
	"context"
	"errors"
	"testing"

	pubapi "github.com/sdcio/data-server/pkg/api"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
)

func TestIntentToInternalTI(t *testing.T) {
	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}
	ds := &Datastore{schemaClient: schemaClient.NewSchemaClientBound(schema, sc)}
	ctx := context.Background()

	content := []byte(`{"interface":[{"name":"ethernet-1/1","description":"d"}]}`)

	t.Run("update", func(t *testing.T) {
		ti, err := ds.IntentToInternalTI(ctx, &pubapi.Intent{
			Name: "i1",
			Spec: pubapi.IntentSpec{Update: &pubapi.Update{
				Priority:          7,
				NonRevertive:      true,
				PreviouslyApplied: true,
				Entries:           []pubapi.UpdateEntry{{Path: "/", Content: content}},
				Deletes:           []string{"/interface[name=ethernet-1/2]"},
				RevertPaths:       []string{"/interface[name=ethernet-1/1]"},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if ti.GetName() != "i1" || ti.GetPriority() != 7 {
			t.Fatalf("name/priority = %q/%d", ti.GetName(), ti.GetPriority())
		}
		if !ti.NonRevertive() || !ti.GetPreviouslyApplied() || ti.GetDeleteFlag() {
			t.Fatal("unexpected flags")
		}
		if len(ti.GetUpdates()) == 0 {
			t.Fatal("expected expanded updates")
		}
		if len(ti.GetRevertPaths()) != 1 {
			t.Fatalf("revert paths = %d", len(ti.GetRevertPaths()))
		}
		if len(ti.GetDeletes().ToPathSlice()) != 1 {
			t.Fatalf("deletes = %d", len(ti.GetDeletes().ToPathSlice()))
		}
	})

	t.Run("delete", func(t *testing.T) {
		ti, err := ds.IntentToInternalTI(ctx, &pubapi.Intent{
			Name: "i2",
			Spec: pubapi.IntentSpec{Delete: &pubapi.Delete{IgnoreNoExist: true, Orphan: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ti.GetDeleteFlag() || !ti.GetDeleteIgnoreNonExisting() || !ti.GetOnlyIntended() {
			t.Fatal("unexpected flags")
		}
		if len(ti.GetUpdates()) != 0 {
			t.Fatal("delete must not carry updates")
		}
	})

	t.Run("revert paths without non-revertive are ignored", func(t *testing.T) {
		ti, err := ds.IntentToInternalTI(ctx, &pubapi.Intent{
			Name: "i3",
			Spec: pubapi.IntentSpec{Update: &pubapi.Update{RevertPaths: []string{"/interface[name=ethernet-1/1]"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(ti.GetRevertPaths()) != 0 {
			t.Fatal("revert paths must be dropped")
		}
	})

	invalid := map[string]pubapi.Intent{
		"both update and delete": {Name: "x", Spec: pubapi.IntentSpec{Update: &pubapi.Update{}, Delete: &pubapi.Delete{}}},
		"bad entry path": {Name: "x", Spec: pubapi.IntentSpec{Update: &pubapi.Update{
			Entries: []pubapi.UpdateEntry{{Path: "/interface[name=", Content: content}},
		}}},
		"bad delete path": {Name: "x", Spec: pubapi.IntentSpec{Update: &pubapi.Update{Deletes: []string{"/interface[name="}}}},
		"bad revert path": {Name: "x", Spec: pubapi.IntentSpec{Update: &pubapi.Update{RevertPaths: []string{"/interface[name="}}}},
	}
	for name, in := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := ds.IntentToInternalTI(ctx, &in)
			if !errors.Is(err, pubapi.ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}
}
