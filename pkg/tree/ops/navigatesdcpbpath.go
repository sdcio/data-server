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
		if len(pathElems) > 1 && pathElems[1].Name == ".." {
			// Consecutive `..` steps: skip to the first ancestor that carries a YANG
			// schema (i.e. the list's key-level node). The recursive call will then
			// handle the next `..` step from that node.
			entry, _ = GetFirstAncestorWithSchema(e)
		} else if e.GetSchema() == nil {
			// KEY-VALUE nodes (schema == nil) are internal tree artifacts that hold
			// key-discriminator values for list instances but carry no YANG schema.
			// In YANG/XPath, `..` means the *YANG-level* parent, which for a list
			// instance is the container that holds the list — not the key-level
			// intermediary node used by SDCIO's tree. Skip all key-value levels via
			// GetFirstAncestorWithSchema (which lands at the key-level node), then
			// take one more step up to reach the actual YANG parent.
			keyLevel, _ := GetFirstAncestorWithSchema(e)
			if keyLevel != nil {
				entry = keyLevel.GetParent()
			}
		} else {
			// Schema-bearing node (container, leaf, or the list's key-level node itself):
			// the immediate parent is the correct YANG-level parent.
			entry = e.GetParent()
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
