package ops

import (
	"context"
	"errors"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ErrDeleteBranchNilEntry is returned when DeleteBranch is called with a nil tree entry.
// Callers must validate the entry before calling.
var ErrDeleteBranchNilEntry = errors.New("DeleteBranch: nil tree entry")

func DeleteBranch(ctx context.Context, e api.Entry, path *sdcpb.Path, owner string) error {
	var entry api.Entry
	var err error

	if e == nil {
		return fmt.Errorf("%w: callers must pass a non-nil entry", ErrDeleteBranchNilEntry)
	}

	if path == nil {
		return deleteBranchInternal(ctx, e, owner)
	}

	// if the relativePath is present, we need to naviagate
	entry, err = NavigateSdcpbPath(ctx, e, path)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	err = DeleteBranch(ctx, entry, nil, owner)
	if err != nil {
		return err
	}

	// need to remove the leafvariants down from entry.
	// however if the path points to a key, which is in fact getting deleted
	// we also need to remove the key, which is the parent. Thats why we do it in this loop
	// which is, forwarding entry to entry.GetParent() as a last step and depending on the remains
	// return continuing to perform the delete forther up in the tree
	// with remains initially set to false, we initially call DeleteSubtree on the referenced entry.
	for entry.CanDeleteBranch(false) {
		// forward the entry pointer to the parent
		// depending on the remains var the DeleteSubtree is again called on that parent entry
		entry = entry.GetParent()
		if entry == nil {
			// we made it all the way up to the root. So we have to return.
			return nil
		}
		// calling DeleteSubtree with the empty string, because it should not delete the owner from the higher level keys,
		// but what it will also do is delete possibly dangling key elements in the tree
		entry.DeleteCanDeleteChilds(true)
	}

	return nil
}

func deleteBranchInternal(ctx context.Context, e api.Entry, owner string) error {
	// delete possibly existing leafvariants for the owner
	e.GetLeafVariants().DeleteByOwner(owner)

	// recurse the call
	for childName, child := range e.GetChildMap().GetAll() {
		if child == nil {
			return fmt.Errorf("%w: child %q in map is nil under %s", ErrDeleteBranchNilEntry, childName, e.SdcpbPath().ToXPath(false))
		}
		err := DeleteBranch(ctx, child, nil, owner)
		if err != nil {
			return err
		}
		if child.CanDeleteBranch(false) {
			e.GetChildMap().DeleteChild(childName)
		}
	}
	return nil
}
