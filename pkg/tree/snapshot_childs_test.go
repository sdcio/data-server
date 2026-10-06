package tree

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func interfaceWithKeyChildren(t *testing.T, n int) api.Entry {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, err := NewTreeRoot(ctx, NewTreeContext(scb, pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))))
	if err != nil {
		t.Fatal(err)
	}
	iface, err := api.NewEntry(ctx, root.Entry, "interface", root.GetTreeContext())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err = api.NewEntry(ctx, iface, fmt.Sprintf("ethernet-1/%d", i+1), iface.GetTreeContext()); err != nil {
			t.Fatal(err)
		}
	}
	return iface
}

func childNames(children []api.Entry) []string {
	names := make([]string, len(children))
	for i, child := range children {
		names[i] = child.PathName()
	}
	slices.Sort(names)
	return names
}

func TestSnapshotChildsSurvivesChildDeletion(t *testing.T) {
	iface := interfaceWithKeyChildren(t, 3)
	snap := iface.SnapshotChilds(types.DescendMethodAll)
	if got, want := childNames(snap), []string{"ethernet-1/1", "ethernet-1/2", "ethernet-1/3"}; !slices.Equal(got, want) {
		t.Fatalf("snapshot names = %v, want %v", got, want)
	}

	iface.GetChildMap().DeleteChild("ethernet-1/2")

	if got, want := childNames(snap), []string{"ethernet-1/1", "ethernet-1/2", "ethernet-1/3"}; !slices.Equal(got, want) {
		t.Fatalf("snapshot after delete = %v, want the pre-delete set %v", got, want)
	}
	if got, want := childNames(iface.SnapshotChilds(types.DescendMethodAll)), []string{"ethernet-1/1", "ethernet-1/3"}; !slices.Equal(got, want) {
		t.Fatalf("live children after delete = %v, want %v", got, want)
	}
}

func TestWalkerRacesChildDeletion(t *testing.T) {
	iface := interfaceWithKeyChildren(t, 64)
	children := iface.SnapshotChilds(types.DescendMethodAll)
	ctx := context.Background()
	tp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	errCh := make(chan error, 1)
	walkerDone := make(chan struct{})
	deleterDone := make(chan struct{})
	go func() {
		defer close(walkerDone)
		for i := 0; i < 100; i++ {
			for _, child := range iface.SnapshotChilds(types.DescendMethodAll) {
				_ = child.PathName()
			}
			p := processors.NewResetFlagsProcessor(&processors.ResetFlagsProcessorParams{DeleteFlag: true, NewFlag: true, UpdateFlag: true})
			if err := p.Run(iface, tp); err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		defer close(deleterDone)
		for round := 0; round < 200; round++ {
			for _, child := range children {
				iface.GetChildMap().DeleteChild(child.PathName())
				iface.GetChildMap().AddOrGet(child)
			}
		}
	}()

	timeout := time.After(30 * time.Second)
	select {
	case <-walkerDone:
	case <-timeout:
		t.Fatal("deadlock: walker did not finish while children were deleted")
	}
	select {
	case <-deleterDone:
	case <-timeout:
		t.Fatal("deadlock: child deletion did not finish while the walker ran")
	}
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
}

func TestDeleteWhileIteratingDoesNotDeadlock(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteCanDeleteChilds", func(t *testing.T) {
		iface := interfaceWithKeyChildren(t, 64)
		done := make(chan struct{})
		go func() {
			iface.DeleteCanDeleteChilds(false)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("deadlock: DeleteCanDeleteChilds did not finish")
		}
		if got := iface.GetChildMap().Length(); got != 0 {
			t.Fatalf("children left after DeleteCanDeleteChilds = %d, want 0", got)
		}
	})

	t.Run("DeleteBranch", func(t *testing.T) {
		iface := interfaceWithKeyChildren(t, 64)
		done := make(chan struct{})
		errCh := make(chan error, 1)
		go func() {
			if err := ops.DeleteBranch(ctx, iface, nil, "owner"); err != nil {
				errCh <- err
			}
			close(done)
		}()
		select {
		case err := <-errCh:
			t.Fatal(err)
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("deadlock: DeleteBranch did not finish")
		}
		if got := iface.GetChildMap().Length(); got != 0 {
			t.Fatalf("children left after DeleteBranch = %d, want 0", got)
		}
	})
}
