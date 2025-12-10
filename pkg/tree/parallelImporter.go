package tree

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/types"
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
	leafListLock    *sync.Map
}

func (s *sharedEntryAttributes) ImportConfig(
	ctx context.Context,
	importerElement importer.ImportConfigAdapterElement,
	intentName string,
	intentPrio int32,
	insertFlags *types.UpdateInsertFlags,
) error {
	p := pool.NewWorkerPool[importTask](ctx, 1)

	p.Start(importHandler)

	// seed root
	if err := p.Submit(importTask{entry: s, importerElement: importerElement, intentName: intentName, intentPrio: intentPrio, insertFlags: insertFlags, treeContext: s.treeContext, leafListLock: &sync.Map{}}); err != nil {
		return err
	}

	// signal we are done seeding external tasks (workers may still submit)
	p.CloseForSubmit()

	// wait for the import to finish (or error)
	return p.Wait()
}

func importHandler(ctx context.Context, task importTask, submit func(importTask) error) error {

	elem := task.entry.PathName()
	_ = elem

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
			return submit(importTask{entry: actual, importerElement: task.importerElement, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext, leafListLock: task.leafListLock})
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
			if err := submit(importTask{entry: child, importerElement: childElt, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext, leafListLock: task.leafListLock}); err != nil {
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
		// for the leaflist, since in XML the leaf list elements are independet elements, we need to make
		// sure that the first element is basically resetting the leaflist and all consecutive elemts are then
		// added to the already resettet leaflist.
		// strategy here is to create a mutex lock it and try to store it in the leafListLock map.
		// if the mutex was then stored, we're the first goroutine and need to reset. If we get a different mutex back
		// and the the loaded var is set to true, we should not reset the list and trxy to lock the returned mutex.

		// create a mutex and lock it
		llMutex := &sync.Mutex{}
		llMutex.Lock()

		// try storing it or load it from leafListLock
		llm, loaded := task.leafListLock.LoadOrStore(task.entry.SdcpbPath().ToXPath(false), llMutex)

		// if it was loaded, we need to lock the loaded mutex
		if loaded {
			llMutex = llm.(*sync.Mutex)
			llMutex.Lock()
		}
		defer llMutex.Unlock()

		var scalarArr *sdcpb.ScalarArray
		mustAdd := false
		var le *LeafEntry
		if loaded {
			le = task.entry.GetLeafVariantEntries().GetByOwner(task.intentName)
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
