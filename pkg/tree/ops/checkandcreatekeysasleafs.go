package ops

import (
	"cmp"
	"context"
	"slices"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func CheckAndCreateKeysAsLeafs(ctx context.Context, e api.Entry, intentName string, prio int32, insertFlag *types.UpdateInsertFlags) error {
	// keys themselfes do not have a schema attached.
	// keys must be added to the last keys level, since that is carrying the list elements data
	// hence if the entry has a schema attached, there is nothing to be done, return.
	if e.GetSchema() != nil {
		return nil
	}

	// get the first ancestor with a schema and how many levels up that is
	ancestor, levelsUp := GetFirstAncestorWithSchema(e)

	// retrieve the container schema
	ancestorContainerSchema := ancestor.GetSchema().GetContainer()
	// if it is not a container, return
	if ancestorContainerSchema == nil {
		return nil
	}

	// if we're in the last level of keys, then we need to add the defaults
	if len(ancestorContainerSchema.Keys) == levelsUp {
		keySorted := make([]*sdcpb.LeafSchema, 0, len(ancestor.GetSchema().GetContainer().Keys))
		// add key leafschemas to slice
		keySorted = append(keySorted, ancestor.GetSchema().GetContainer().Keys...)
		// sort keySorted slice
		slices.SortFunc(keySorted, func(a, b *sdcpb.LeafSchema) int {
			return cmp.Compare(b.Name, a.Name)
		})

		// iterate through the keys
		var item api.Entry = e

		// construct the key path
		// doing so outside the loop to reuse
		path := &sdcpb.Path{
			IsRootBased: false,
		}

		for _, k := range keySorted {
			child, entryExists := e.GetChildMap().GetEntry(k.Name)
			// if the key Leaf exists continue with next key
			if entryExists {
				// if it exists, we need to check that the entry for the owner exists.
				le := child.GetLeafVariants().GetByOwner(intentName)
				if le != nil {
					le.DropDeleteFlag()
					// continue with parent Entry BEFORE continuing the loop
					item = item.GetParent()
					continue
				}
			}

			// convert the key value to the schema defined Typed_Value
			tv, err := sdcpb.TVFromString(k.GetType(), item.PathName(), 0)
			if err != nil {
				return err
			}
			if !entryExists {
				// create a new entry
				child, err = api.NewEntry(ctx, e, k.Name, e.GetTreeContext())

				if err != nil {
					return err
				}
			}
			// add the new child entry to s
			err = e.AddChild(ctx, child)
			if err != nil {
				return err
			}

			// Add the update to the tree
			_, err = AddUpdateRecursive(ctx, child, path, types.NewUpdate(nil, tv, prio, intentName, 0), insertFlag)
			if err != nil {
				return err
			}

			// continue with parent Entry
			item = item.GetParent()
		}
	}
	return nil
}
