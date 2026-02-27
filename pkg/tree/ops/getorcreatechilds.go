package ops

import (
	"context"
	"sort"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func GetOrCreateChilds(ctx context.Context, e api.Entry, path *sdcpb.Path) (api.Entry, error) {
	if path == nil || len(path.Elem) == 0 {
		return e, nil
	}

	var current api.Entry = e
	for i, pe := range path.Elem {
		// Step 1: Find or create the child for the path element name
		newCurrent, exists := current.GetChilds(types.DescendMethodAll)[pe.Name]
		if !exists {
			var err error
			child, err := api.NewEntry(ctx, current, pe.Name, e.GetTreeContext())
			if err != nil {
				return nil, err
			}
			if err := current.AddChild(ctx, child); err != nil {
				return nil, err
			}
			newCurrent = child
		}
		current = newCurrent

		// sort keys
		keys := make([]string, 0, len(pe.Key))
		for key := range pe.Key {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Step 2: For each key, find or create the key child
		for _, key := range keys {
			newCurrent, exists = current.GetChilds(types.DescendMethodAll)[pe.Key[key]]
			if !exists {
				var err error
				keyChild, err := api.NewEntry(ctx, current, pe.Key[key], e.GetTreeContext())
				if err != nil {
					return nil, err
				}
				if err := current.AddChild(ctx, keyChild); err != nil {
					return nil, err
				}
				newCurrent = keyChild
			}
			current = newCurrent
		}

		// If this is the last PathElem, return the current node
		if i == len(path.Elem)-1 {
			return current, nil
		}
	}

	return current, nil
}

// AddUpdateRecursive recursively adds the given cache.Update to the tree. Thereby creating all the entries along the path.
// if the entries along th path already exist, the existing entries are called to add the Update.
func AddUpdateRecursive(ctx context.Context, e api.Entry, path *sdcpb.Path, u *types.Update, flags *types.UpdateInsertFlags) (api.Entry, error) {
	var err error
	relPath := path

	if path.IsRootBased && !e.IsRoot() {
		// calculate the relative path for the add
		relPath, err = path.AbsToRelativePath(e.SdcpbPath())
		if err != nil {
			return nil, err
		}
	}
	return AddUpdateRecursiveInternal(ctx, e, relPath, 0, u, flags)
}

func AddUpdateRecursiveInternal(ctx context.Context, s api.Entry, path *sdcpb.Path, idx int, u *types.Update, flags *types.UpdateInsertFlags) (api.Entry, error) {
	// make sure all the keys are also present as leafs
	err := CheckAndCreateKeysAsLeafs(ctx, s, u.Owner(), u.Priority(), flags)
	if err != nil {
		return nil, err
	}
	// end of path reached, add LeafEntry
	// continue with recursive add otherwise
	if path == nil || len(path.GetElem()) == 0 || idx >= len(path.GetElem()) {
		// delegate update handling to leafVariants
		s.GetLeafVariants().Add(api.NewLeafEntry(u, flags, s))
		return s, nil
	}

	var e api.Entry
	var x api.Entry = s
	var exists bool
	for name := range path.GetElem()[idx].PathElemNames() {
		if e, exists = x.GetChilds(types.DescendMethodAll)[name]; !exists {
			newE, err := api.NewEntry(ctx, x, name, s.GetTreeContext())
			if err != nil {
				return nil, err
			}
			err = x.AddChild(ctx, newE)
			if err != nil {
				return nil, err
			}
			e = newE
		}
		x = e
	}

	return AddUpdateRecursiveInternal(ctx, x, path, idx+1, u, flags)
}
