package tree

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"

	treeimporter "github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type importConfigTask struct {
	entry           Entry
	importerElement treeimporter.ImportConfigAdapterElement
	params          *ImportConfigProcessorParams
}

type ImportConfigProcessorParams struct {
	intentName   string
	intentPrio   int32
	insertFlags  *types.UpdateInsertFlags
	treeContext  *TreeContext
	leafListLock *sync.Map
	stats        *types.ImportStats
}

func NewParameters(
	intentName string,
	intentPrio int32,
	insertFlags *types.UpdateInsertFlags,
	treeContext *TreeContext,
	leafListLock *sync.Map,
	stats *types.ImportStats,
) *ImportConfigProcessorParams {
	return &ImportConfigProcessorParams{
		intentName:   intentName,
		intentPrio:   intentPrio,
		insertFlags:  insertFlags,
		treeContext:  treeContext,
		leafListLock: leafListLock,
		stats:        stats,
	}
}

type ImportConfigProcessor struct {
	importer    treeimporter.ImportConfigAdapter
	insertFlags *types.UpdateInsertFlags
	stats       *types.ImportStats
}

func NewImportConfigProcessor(importer treeimporter.ImportConfigAdapter, insertFlags *types.UpdateInsertFlags) *ImportConfigProcessor {
	return &ImportConfigProcessor{
		importer:    importer,
		insertFlags: insertFlags,
		stats:       types.NewImportStats(),
	}
}

func (p *ImportConfigProcessor) GetStats() *types.ImportStats {
	return p.stats
}

func (p *ImportConfigProcessor) Run(ctx context.Context, e Entry, poolFactory pool.VirtualPoolFactory) error {
	// set actual owner
	e.GetTreeContext().SetActualOwner(p.importer.GetName())

	// store non revertive info
	e.GetTreeContext().nonRevertiveInfo[p.importer.GetName()] = p.importer.GetNonRevertive()

	// store explicit deletes
	e.GetTreeContext().explicitDeletes.Add(p.importer.GetName(), p.importer.GetPriority(), p.importer.GetDeletes())

	workerPool := poolFactory.NewVirtualPool(pool.VirtualFailFast)

	t := importConfigTask{
		entry:           e,
		importerElement: p.importer,
		params:          NewParameters(p.importer.GetName(), p.importer.GetPriority(), p.insertFlags, e.GetTreeContext(), &sync.Map{}, p.stats),
	}

	if err := workerPool.Submit(t); err != nil {
		workerPool.CloseAndWait()
		return err
	}

	workerPool.CloseAndWait()

	if err := workerPool.FirstError(); err != nil {
		return err
	}
	// TODO: support tolerant mode? Processor usually decides what to return based on pool mode,
	// but FirstError() works for fail-fast. For tolerant, we might want Errors().
	// But ImportConfig usually stops on error?
	return nil
}

func (task importConfigTask) Run(ctx context.Context, submit func(pool.Task) error) error {

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
					keyChild, err = NewEntry(ctx, actual, kv, task.params.treeContext)
					if err != nil {
						return err
					}
				}
				actual = keyChild
			}
			// submit resolved entry with same adapter element
			// return importHandler(ctx, importTask{entry: actual, importerElement: task.importerElement, intentName: task.intentName, intentPrio: task.intentPrio, insertFlags: task.insertFlags, treeContext: task.treeContext}, submit)
			return submit(importConfigTask{entry: actual, importerElement: task.importerElement, params: task.params})
		}

		// presence container or children
		elems := task.importerElement.GetElements()
		if len(elems) == 0 {
			schem := task.entry.GetSchema().GetContainer()
			if schem != nil && schem.IsPresence {
				tv := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{EmptyVal: &emptypb.Empty{}}}
				upd := types.NewUpdate(task.entry, tv, task.params.intentPrio, task.params.intentName, 0)
				task.entry.GetLeafVariantEntries().Add(NewLeafEntry(upd, task.params.insertFlags, task.entry))
			}
			return nil
		}

		// submit each child
		for _, childElt := range elems {
			child, exists := task.entry.GetChild(childElt.GetName())
			if !exists {
				var err error
				child, err = NewEntry(ctx, task.entry, childElt.GetName(), task.params.treeContext)
				if err != nil {
					return fmt.Errorf("error inserting %s at %s: %w", childElt.GetName(), task.entry.SdcpbPath().ToXPath(false), err)
				}
			}
			// need to process Leaflist childs in this goroutine to avois reordering
			switch child.GetSchema().GetSchema().(type) {
			case *sdcpb.SchemaElem_Leaflist:
				err := importConfigTask{entry: child, importerElement: childElt, params: task.params}.Run(ctx, submit)
				if err != nil {
					return err
				}
			default:
				if err := submit(importConfigTask{entry: child, importerElement: childElt, params: task.params}); err != nil {
					return err
				}
			}
		}
		return nil

	case *sdcpb.SchemaElem_Field:
		tv, err := task.importerElement.GetTVValue(ctx, x.Field.GetType())
		if err != nil {
			return err
		}
		upd := types.NewUpdate(task.entry, tv, task.params.intentPrio, task.params.intentName, 0)
		task.entry.GetLeafVariantEntries().AddWithStats(NewLeafEntry(upd, task.params.insertFlags, task.entry), task.params.stats)
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
		llm, loaded := task.params.leafListLock.LoadOrStore(task.entry.SdcpbPath().ToXPath(false), llMutex)

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
			le = task.entry.GetLeafVariantEntries().GetByOwner(task.params.intentName)
			scalarArr = le.Value().GetLeaflistVal()
		} else {
			le = NewLeafEntry(nil, task.params.insertFlags, task.entry)
			mustAdd = true
			scalarArr = &sdcpb.ScalarArray{Element: []*sdcpb.TypedValue{}}
		}

		tv, err := task.importerElement.GetTVValue(ctx, x.Leaflist.GetType())
		if err != nil {
			return err
		}
		if tv.GetLeaflistVal() == nil {
			scalarArr.Element = append(scalarArr.Element, tv)
			tv = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: scalarArr}}
		}
		le.Update = types.NewUpdate(task.entry, tv, task.params.intentPrio, task.params.intentName, 0)
		if mustAdd {
			task.entry.GetLeafVariantEntries().Add(le)
		}
		return nil
	default:
		return nil
	}
}
