package tree

import (
	"context"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func BlameConfig(ctx context.Context, e Entry, includeDefaults bool, pool pool.VirtualPoolI) (*sdcpb.BlameTreeElement, error) {
	dropChan := make(chan *DropBlameChild, 10)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	// execute the deletes in a seperate single channel
	go func(dC <-chan *DropBlameChild) {
		for elem := range dC {
			elem.Exec()
		}
		wg.Done()
	}(dropChan)

	blameTask := NewBlameConfigTask(e, dropChan, includeDefaults)
	err := pool.Submit(blameTask)
	if err != nil {
		return nil, err
	}
	// close pool for additional external submission
	pool.CloseForSubmit()
	// wait for the pool to run dry
	pool.Wait()
	// close the dropChan channel
	close(dropChan)
	wg.Wait()

	return blameTask.self, pool.FirstError()
}

type BlameConfigTask struct {
	includeDefaults bool
	parent          *sdcpb.BlameTreeElement
	self            *sdcpb.BlameTreeElement
	selfEntry       Entry
	dropChan        chan<- *DropBlameChild
}

func NewBlameConfigTask(e Entry, dropChan chan<- *DropBlameChild, includeDefaults bool) *BlameConfigTask {
	return &BlameConfigTask{
		includeDefaults: includeDefaults,
		parent:          nil,
		self:            &sdcpb.BlameTreeElement{},
		selfEntry:       e,
		dropChan:        dropChan,
	}
}

func (t *BlameConfigTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	t.self.Name = t.selfEntry.PathName()
	if t.selfEntry.IsRoot() {
		t.self.Name = "root"
	}

	// process Value
	highestLe := t.selfEntry.GetLeafVariantEntries().GetHighestPrecedence(false, true, true)
	if highestLe != nil {
		if highestLe.Update.Owner() != DefaultsIntentName || t.includeDefaults {
			t.self.SetValue(highestLe.Update.Value()).SetOwner(highestLe.Update.Owner())

			// check if running equals the expected
			runningLe := t.selfEntry.GetLeafVariantEntries().GetRunning()
			if runningLe != nil {
				if !proto.Equal(runningLe.Update.Value(), highestLe.Update.Value()) {
					t.self.SetDeviationValue(runningLe.Value())
				}
			}
		} else {
			// if it is default but no default is meant to be returned
			t.dropChan <- &DropBlameChild{parent: t.parent, dropElem: t.self}
		}
	}

	for _, childEntry := range t.selfEntry.GetChilds(DescendMethodActiveChilds) {
		child := &sdcpb.BlameTreeElement{Name: childEntry.PathName()}
		t.self.AddChild(child)

		// create a new task for each child
		task := &BlameConfigTask{
			includeDefaults: t.includeDefaults,
			parent:          t.self,
			self:            child,
			selfEntry:       childEntry,
			dropChan:        t.dropChan,
		}
		// submit the task
		if err := submit(task); err != nil {
			return err
		}
	}

	return nil
}

type DropBlameChild struct {
	parent   *sdcpb.BlameTreeElement
	dropElem *sdcpb.BlameTreeElement
}

func (d *DropBlameChild) Exec() {
	// from parent drop the child dropElem
	index := -1
	for i, child := range d.parent.GetChilds() {
		if child == d.dropElem {
			index = i
			break
		}
	}
	if index != -1 {
		d.parent.Childs = append(d.parent.Childs[:index], d.parent.Childs[index+1:]...)
	}
}
