package tree

import (
	"context"
	"fmt"
	"runtime"
	"slices"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type importTask struct {
	entry           Entry
	importerElement importer.ImportConfigAdapterElement
	intentName      string
	intentPrio      int32
	insertFlags     *types.UpdateInsertFlags
	treeContext     *TreeContext
}

func (s *sharedEntryAttributes) ImportConfig(
	ctx context.Context,
	importerElement importer.ImportConfigAdapterElement,
	intentName string,
	intentPrio int32,
	insertFlags *types.UpdateInsertFlags,
) error {
	p := utils.NewWorkerPool[importTask](ctx, runtime.NumCPU())

	p.Start(importHandler)

	// seed root
	if err := p.Submit(importTask{entry: s, importerElement: importerElement, intentName: intentName, intentPrio: intentPrio, insertFlags: insertFlags, treeContext: s.treeContext}); err != nil {
		return err
	}

	// signal we are done seeding external tasks (workers may still submit)
	p.CloseForSubmit()

	// wait for the import to finish (or error)
	return p.Wait()
}

func importHandler(ctx context.Context, task importTask, submit func(importTask) error) error {
	switch x := task.entry.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container, nil:
		// keyed container: handle keys sequentially
		if len(task.entry.GetSchema().GetContainer().GetKeys()) > 0 {
			var exists bool
			var actual Entry = task.entry
			var keyChild Entry

			keys := task.entry.GetSchemaKeys()
			slices.Sort(keys)
			for _, k := range keys {
				ktrans := task.importerElement.GetElement(k)
				if ktrans == nil {
					return fmt.Errorf("unable to find key attribute %s under %s", k, task.entry.SdcpbPath().ToXPath(false))
				}
				kv, err := ktrans.GetKeyValue()
				if err != nil {
					return err
				}
				if keyChild, exists = actual.GetChild(kv); !exists {
					keyChild, err = newEntry(ctx, actual, kv, task.treeContext)
					if err != nil {
						return err
					}
				}
				actual = keyChild
			}
			// submit resolved entry with same adapter element
			// return importHandler(ctx, importTask{entry: actual, importerElement: task.importerElement, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext}, submit)
			return submit(importTask{entry: actual, importerElement: task.importerElement, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext})
		}

		// presence container or children
		elems := task.importerElement.GetElements()
		if len(elems) == 0 {
			schem := task.entry.GetSchema().GetContainer()
			if schem != nil && schem.IsPresence {
				tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{EmptyVal: &emptypb.Empty{}}}
				upd := types.NewUpdate(task.entry.SdcpbPath(), tv, task.intentPrio, task.intentName, 0)
				task.entry.GetLeafVariantEntries().Add(NewLeafEntry(upd, task.insertFlags, task.entry))
			}
			return nil
		}

		// submit each child (no external locking per your guarantee)
		for _, childElt := range elems {
			child, exists := task.entry.GetChild(childElt.GetName())
			if !exists {
				var err error
				child, err = newEntry(ctx, task.entry, childElt.GetName(), task.treeContext)
				if err != nil {
					return fmt.Errorf("error inserting %s at %s: %w", childElt.GetName(), task.entry.SdcpbPath().ToXPath(false), err)
				}
			}
			if err := submit(importTask{entry: child, importerElement: childElt, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext}); err != nil {
				return err
			}
		}
		return nil

	case *sdcpb.SchemaElem_Field:
		tv, err := task.importerElement.GetTVValue(x.Field.GetType())
		if err != nil {
			return err
		}
		upd := types.NewUpdate(task.entry.SdcpbPath(), tv, task.intentPrio, task.intentName, 0)
		task.entry.GetLeafVariantEntries().Add(NewLeafEntry(upd, task.insertFlags, task.entry))
		return nil

	case *sdcpb.SchemaElem_Leaflist:
		var scalarArr *sdcpb.ScalarArray
		mustAdd := false
		le := task.entry.GetLeafVariantEntries().GetByOwner(task.intentName)
		if le != nil {
			scalarArr = le.Value().GetLeaflistVal()
		} else {
			le = NewLeafEntry(nil, task.insertFlags, task.entry)
			mustAdd = true
			scalarArr = &sdcpb.ScalarArray{Element: []*sdcpb.TypedValue{}}
		}

		tv, err := task.importerElement.GetTVValue(x.Leaflist.GetType())
		if err != nil {
			return err
		}
		if tv.GetLeaflistVal() == nil {
			scalarArr.Element = append(scalarArr.Element, tv)
			tv = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: scalarArr}}
		}
		le.Update = types.NewUpdate(task.entry.SdcpbPath(), tv, task.intentPrio, task.intentName, 0)
		if mustAdd {
			task.entry.GetLeafVariantEntries().Add(le)
		}
		return nil
	default:
		return nil
	}
}
