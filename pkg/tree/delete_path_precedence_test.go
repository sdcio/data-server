package tree

import (
	"context"
	"runtime"
	"slices"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

const deletePathDescXPath = "/interface[name=ethernet-1/1]/description"

func TestDeletePathPrecedence(t *testing.T) {
	ctx := context.Background()
	deleteOwner := "delete-owner"
	deletePrioJustAboveRunning := consts.RunningValuesPrio - 1
	otherOwner := "other-owner"
	descPath := sdcpb.NewPathSet().AddPath(&sdcpb.Path{
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
			sdcpb.NewPathElem("description", nil),
		},
	})

	tests := []struct {
		name                        string
		load                        func(t *testing.T, root *RootEntry)
		deleteOwner                 string
		deletePrio                  int32
		wantHighestOwner            string
		wantHighestValue            string
		wantOwnerPriority           int32
		wantOwnerValue              string
		wantHighestIsExplicitDelete bool
		wantDeviceDelete            bool
	}{
		{
			// The owner already has a real variant at priority 50. Finish-insertion
			// converts that variant in place: the value stays "owner-desc" and the
			// priority stays 50. The delete-path priority (200) is not written onto
			// it, so the converted variant still outranks the other intent at 100.
			// Applying 200 instead would lose to that other intent.
			name: "owner has real variant at same leaf — converted in place, priority and value kept",
			load: func(t *testing.T, root *RootEntry) {
				t.Helper()
				mustLoadIfaceDesc(t, ctx, root, consts.RunningIntentName, consts.RunningValuesPrio, "running-desc", testhelper.FlagsExisting)
				mustLoadIfaceDesc(t, ctx, root, deleteOwner, 50, "owner-desc", testhelper.FlagsNew)
				mustLoadIfaceDesc(t, ctx, root, otherOwner, 100, "other-desc", testhelper.FlagsNew)
			},
			deleteOwner:                 deleteOwner,
			deletePrio:                  200,
			wantHighestOwner:            deleteOwner,
			wantHighestValue:            "owner-desc",
			wantOwnerPriority:           50,
			wantOwnerValue:              "owner-desc",
			wantHighestIsExplicitDelete: true,
			wantDeviceDelete:            true,
		},
		{
			name: "delete-path wins over Running",
			load: func(t *testing.T, root *RootEntry) {
				t.Helper()
				mustLoadIfaceDesc(t, ctx, root, consts.RunningIntentName, consts.RunningValuesPrio, "running-desc", testhelper.FlagsExisting)
			},
			deleteOwner:                 deleteOwner,
			deletePrio:                  deletePrioJustAboveRunning,
			wantHighestOwner:            deleteOwner,
			wantHighestValue:            "",
			wantOwnerPriority:           deletePrioJustAboveRunning,
			wantOwnerValue:              "",
			wantHighestIsExplicitDelete: true,
			wantDeviceDelete:            true,
		},
		{
			name: "delete-path wins over defaults",
			load: func(t *testing.T, root *RootEntry) {
				t.Helper()
				mustLoadIfaceDesc(t, ctx, root, consts.RunningIntentName, consts.RunningValuesPrio, "running-desc", testhelper.FlagsExisting)
				mustLoadIfaceDesc(t, ctx, root, consts.DefaultsIntentName, consts.DefaultValuesPrio, "default-desc", testhelper.FlagsExisting)
			},
			deleteOwner:                 deleteOwner,
			deletePrio:                  deletePrioJustAboveRunning,
			wantHighestOwner:            deleteOwner,
			wantHighestValue:            "",
			wantOwnerPriority:           deletePrioJustAboveRunning,
			wantOwnerValue:              "",
			wantHighestIsExplicitDelete: true,
			wantDeviceDelete:            true,
		},
		{
			name: "delete-path without Running does not issue a device delete",
			load: func(t *testing.T, root *RootEntry) {
				t.Helper()
				mustLoadIfaceDesc(t, ctx, root, consts.DefaultsIntentName, consts.DefaultValuesPrio, "default-desc", testhelper.FlagsExisting)
			},
			deleteOwner:                 deleteOwner,
			deletePrio:                  deletePrioJustAboveRunning,
			wantHighestOwner:            deleteOwner,
			wantHighestValue:            "",
			wantOwnerPriority:           deletePrioJustAboveRunning,
			wantOwnerValue:              "",
			wantHighestIsExplicitDelete: true,
			wantDeviceDelete:            false,
		},
		{
			name: "other intent real value wins over delete-path",
			load: func(t *testing.T, root *RootEntry) {
				t.Helper()
				mustLoadIfaceDesc(t, ctx, root, consts.RunningIntentName, consts.RunningValuesPrio, "running-desc", testhelper.FlagsExisting)
				mustLoadIfaceDesc(t, ctx, root, otherOwner, 50, "intended-desc", testhelper.FlagsNew)
			},
			deleteOwner:                 deleteOwner,
			deletePrio:                  deletePrioJustAboveRunning,
			wantHighestOwner:            otherOwner,
			wantHighestValue:            "intended-desc",
			wantOwnerPriority:           deletePrioJustAboveRunning,
			wantOwnerValue:              "",
			wantHighestIsExplicitDelete: false,
			wantDeviceDelete:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
			if err != nil {
				t.Fatal(err)
			}
			root, err := NewTreeRoot(ctx, NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))))
			if err != nil {
				t.Fatal(err)
			}

			tt.load(t, root)
			root.GetTreeContext().ExplicitDeletes().Add(tt.deleteOwner, tt.deletePrio, descPath)

			if err := root.FinishInsertionPhase(ctx); err != nil {
				t.Fatal(err)
			}

			const (
				onlyNewOrUpdated      = false
				includeDefaults       = true
				includeExplicitDelete = true
			)
			highestIncludingExplicitDelete := leafAt(ops.GetHighestPrecedence(root.Entry, onlyNewOrUpdated, includeDefaults, includeExplicitDelete), deletePathDescXPath)
			if highestIncludingExplicitDelete == nil {
				t.Fatalf("no highest-precedence variant at %s", deletePathDescXPath)
			}
			if highestIncludingExplicitDelete.Owner() != tt.wantHighestOwner {
				t.Errorf("highest owner = %q, want %q", highestIncludingExplicitDelete.Owner(), tt.wantHighestOwner)
			}
			if got := highestIncludingExplicitDelete.Value().GetStringVal(); got != tt.wantHighestValue {
				t.Errorf("highest value = %q, want %q", got, tt.wantHighestValue)
			}
			if highestIncludingExplicitDelete.GetExplicitDeleteFlag() != tt.wantHighestIsExplicitDelete {
				t.Errorf("highest IsExplicitDelete = %v, want %v", highestIncludingExplicitDelete.GetExplicitDeleteFlag(), tt.wantHighestIsExplicitDelete)
			}

			ownerLeaf := leafAt(ops.LeafsOfOwner(root.Entry, tt.deleteOwner), deletePathDescXPath)
			if ownerLeaf == nil {
				t.Fatalf("delete owner has no variant at %s", deletePathDescXPath)
			}
			if ownerLeaf.Priority() != tt.wantOwnerPriority {
				t.Errorf("delete owner priority = %d, want %d", ownerLeaf.Priority(), tt.wantOwnerPriority)
			}
			if got := ownerLeaf.Value().GetStringVal(); got != tt.wantOwnerValue {
				t.Errorf("delete owner value = %q, want %q", got, tt.wantOwnerValue)
			}
			if !ownerLeaf.GetExplicitDeleteFlag() {
				t.Errorf("delete owner variant at %s is not an explicit delete", deletePathDescXPath)
			}

			effective := leafAt(root.GetHighestPrecedence(false), deletePathDescXPath)
			if tt.wantHighestIsExplicitDelete {
				if effective != nil {
					t.Errorf("effective highest at %s = owner %q value %q, want none (explicit delete)", deletePathDescXPath, effective.Owner(), effective.Value().ToString())
				}
			} else if effective == nil {
				t.Errorf("effective highest at %s is nil, want owner %q", deletePathDescXPath, tt.wantHighestOwner)
			} else if effective.Owner() != tt.wantHighestOwner {
				t.Errorf("effective highest owner = %q, want %q", effective.Owner(), tt.wantHighestOwner)
			}

			gotDeletes := deleteXPaths(t, root)
			hasDelete := slices.Contains(gotDeletes, deletePathDescXPath)
			if hasDelete != tt.wantDeviceDelete {
				t.Errorf("device deletes contain %s = %v, want %v; deletes = %v", deletePathDescXPath, hasDelete, tt.wantDeviceDelete, gotDeletes)
			}
		})
	}
}

func mustLoadIfaceDesc(t *testing.T, ctx context.Context, root *RootEntry, owner string, prio int32, desc string, flags *types.UpdateInsertFlags) {
	t.Helper()
	cfg := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{
			"ethernet-1/1": {
				Name:        ygot.String("ethernet-1/1"),
				Description: ygot.String(desc),
			},
		},
	}
	if _, err := testhelper.LoadYgotStructIntoTreeRoot(ctx, cfg, root.Entry, owner, prio, false, flags); err != nil {
		t.Fatal(err)
	}
}

func leafAt(lvs api.LeafVariantSlice, xpath string) *api.LeafEntry {
	for _, le := range lvs {
		if le.SdcpbPath().ToXPath(false) == xpath {
			return le
		}
	}
	return nil
}

func deleteXPaths(t *testing.T, root *RootEntry) []string {
	t.Helper()
	dels, err := root.GetDeletes(true)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(dels))
	for _, d := range dels {
		out = append(out, d.SdcpbPath().ToXPath(false))
	}
	slices.Sort(out)
	return out
}
