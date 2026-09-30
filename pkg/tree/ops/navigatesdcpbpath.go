package ops

import (
	"context"
	"errors"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ErrNavigateSdcpbPathNotFound is returned (wrapped) when tree navigation cannot
// resolve a path segment. Delete-by-path callers (e.g. DeleteBranch) treat this as idempotent.
var ErrNavigateSdcpbPathNotFound = errors.New("path not found in tree")

func NavigateSdcpbPath(ctx context.Context, e api.Entry, path *sdcpb.Path) (api.Entry, error) {
	return navigateSdcpbPath(ctx, e, path, 0)
}

func navigateSdcpbPath(ctx context.Context, e api.Entry, path *sdcpb.Path, elemIndex int) (api.Entry, error) {
	if e == nil {
		return nil, fmt.Errorf("%w: nil entry", ErrNavigateSdcpbPathNotFound)
	}
	pathElems := path.GetElem()
	var err error
	if len(pathElems) == 0 {
		return e, nil
	}

	if path.IsRootBased {
		return navigateSdcpbPath(ctx, GetRoot(e), path.DeepCopy().SetIsRootBased(false), 0)
	}

	switch pathElems[0].Name {
	case ".":
		return navigateSdcpbPath(ctx, e, path.CopyAndRemoveFirstPathElem(), elemIndex+1)
	case "..":
		var entry api.Entry
		entry = e.GetParent()
		// we need to skip key levels in the tree
		// if the next path element is again .. we need to skip key values that are present in the tree
		// If it is a sub-entry instead, we need to stay in the brach that is defined by the key values
		// hence only delegate the call to the parent

		if len(pathElems) > 1 && pathElems[1].Name == ".." {
			entry, _ = GetFirstAncestorWithSchema(e)
		}
		if entry == nil {
			return nil, fmt.Errorf("%w: parent is nil at %q", ErrNavigateSdcpbPathNotFound, path.ToXPath(false))
		}
		return navigateSdcpbPath(ctx, entry, path.CopyAndRemoveFirstPathElem(), elemIndex+1)
	default:
		child, exists := api.LookupChild(e.GetChilds(types.DescendMethodActiveChilds), pathElems[0], path, elemIndex)
		if !exists {
			pth := &sdcpb.Path{Elem: pathElems}
			return nil, fmt.Errorf("%w: reached %v but child %v does not exist", ErrNavigateSdcpbPathNotFound, e.SdcpbPath().ToXPath(false), pth.ToXPath(false))
		}

		for v := range pathElems[0].PathElemNamesKeysOnly() {
			// make sure to only skip the first element
			child, err = navigateSdcpbPath(ctx, child, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(v, nil)}}, 0)
			if err != nil {
				return nil, err
			}
		}

		return navigateSdcpbPath(ctx, child, path.CopyAndRemoveFirstPathElem(), elemIndex+1)
	}
}
