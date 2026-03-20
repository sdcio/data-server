package ops

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func NavigateSdcpbPath(ctx context.Context, e api.Entry, path *sdcpb.Path) (api.Entry, error) {
	pathElems := path.GetElem()
	var err error
	if len(pathElems) == 0 {
		return e, nil
	}

	if path.IsRootBased {
		return NavigateSdcpbPath(ctx, GetRoot(e), path.DeepCopy().SetIsRootBased(false))
	}

	switch pathElems[0].Name {
	case ".":
		return NavigateSdcpbPath(ctx, e, path.CopyAndRemoveFirstPathElem())
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
		return NavigateSdcpbPath(ctx, entry, path.CopyAndRemoveFirstPathElem())
	default:
		child, exists := e.GetChilds(types.DescendMethodActiveChilds)[pathElems[0].Name]
		if !exists {
			pth := &sdcpb.Path{Elem: pathElems}
			return nil, fmt.Errorf("navigating tree, reached %v but child %v does not exist, trying to load defaults yielded %v", e.SdcpbPath().ToXPath(false), pth.ToXPath(false), err)
		}

		for v := range pathElems[0].PathElemNamesKeysOnly() {
			// make sure to only skip the first element
			child, err = NavigateSdcpbPath(ctx, child, &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem(v, nil)}})
			if err != nil {
				return nil, err
			}
		}

		return NavigateSdcpbPath(ctx, child, path.CopyAndRemoveFirstPathElem())
	}
}
