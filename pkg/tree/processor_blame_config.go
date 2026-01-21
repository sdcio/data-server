package tree

import (
	"context"
	"errors"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type BlameConfigProcessor struct {
	config *BlameConfigProcessorConfig
}

func NewBlameConfigProcessor(config *BlameConfigProcessorConfig) *BlameConfigProcessor {
	return &BlameConfigProcessor{
		config: config,
	}
}

type BlameConfigProcessorConfig struct {
	includeDefaults bool
}

func NewBlameConfigProcessorConfig(includeDefaults bool) *BlameConfigProcessorConfig {
	return &BlameConfigProcessorConfig{
		includeDefaults: includeDefaults,
	}
}

// Run processes the entry tree starting from e, building a blame tree showing which owner
// (intent) is responsible for each configuration value. The pool parameter should be
// VirtualFailFast to stop on first error.
// Returns the blame tree structure and any error encountered.
func (p *BlameConfigProcessor) Run(ctx context.Context, e Entry, pool pool.VirtualPoolI) (*sdcpb.BlameTreeElement, error) {
	dropChan := make(chan *DropBlameChild, 10)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	// Execute the deletes in a separate goroutine
	go func(dC <-chan *DropBlameChild) {
		for elem := range dC {
			elem.Exec()
		}
		wg.Done()
	}(dropChan)

	blameTask := NewBlameConfigTask(e, dropChan, p.config)
	if err := pool.Submit(blameTask); err != nil {
		// Clean up pool and channels even on early error
		pool.CloseAndWait()
		close(dropChan)
		wg.Wait()
		return nil, err
	}

	// Close pool and wait for all tasks to complete before checking errors
	pool.CloseAndWait()

	// Close the dropChan channel and wait for cleanup goroutine
	close(dropChan)
	wg.Wait()

	// Return first error for fail-fast mode, or combined errors for tolerant mode
	if errs := pool.Errors(); len(errs) > 0 {
		return blameTask.self, errors.Join(errs...)
	}
	return blameTask.self, pool.FirstError()
}

type BlameConfigTask struct {
	config    *BlameConfigProcessorConfig
	parent    *sdcpb.BlameTreeElement
	self      *sdcpb.BlameTreeElement
	selfEntry Entry
	dropChan  chan<- *DropBlameChild
}

func NewBlameConfigTask(e Entry, dropChan chan<- *DropBlameChild, c *BlameConfigProcessorConfig) *BlameConfigTask {
	return &BlameConfigTask{
		config:    c,
		parent:    nil,
		self:      &sdcpb.BlameTreeElement{},
		selfEntry: e,
		dropChan:  dropChan,
	}
}

func (t *BlameConfigTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	t.self.Name = t.selfEntry.PathName()
	if t.selfEntry.IsRoot() {
		t.self.Name = "root"
	}

	// process Value
	highestLe := t.selfEntry.GetLeafVariantEntries().GetHighestPrecedence(false, true, true)
	if highestLe != nil {
		if highestLe.Update.Owner() != DefaultsIntentName || t.config.includeDefaults {
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

		// Create a new task for each child
		task := &BlameConfigTask{
			config:    t.config,
			parent:    t.self,
			self:      child,
			selfEntry: childEntry,
			dropChan:  t.dropChan,
		}
		// Submit may fail if pool is closed or fail-fast error occurred
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
