package processors

import (
	"context"
	"errors"

	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type BlameConfigProcessor struct {
	context *BlameConfigProcessorParams
}

func NewBlameConfigProcessor(params *BlameConfigProcessorParams) *BlameConfigProcessor {
	return &BlameConfigProcessor{
		context: params,
	}
}

type BlameConfigProcessorParams struct {
	IncludeDefaults bool
}

// Run processes the entry tree starting from e, building a blame tree showing which owner
// (intent) is responsible for each configuration value. The pool parameter should be
// VirtualFailFast to stop on first error.
// Returns the blame tree structure and any error encountered.
func (p *BlameConfigProcessor) Run(ctx context.Context, e api.Entry, poolFactory pool.VirtualPoolFactory) (*sdcpb.BlameTreeElement, error) {
	pool := poolFactory.NewVirtualPool(pool.VirtualTolerant)
	blameTask := NewBlameConfigTask(e, p.context)
	if err := pool.Submit(blameTask); err != nil {
		// Clean up pool even on early error
		pool.CloseAndWait()
		return nil, err
	}

	// Close pool and wait for all tasks to complete before checking errors
	pool.CloseAndWait()

	// Return first error for fail-fast mode, or combined errors for tolerant mode
	if errs := pool.Errors(); len(errs) > 0 {
		return blameTask.self, errors.Join(errs...)
	}
	return blameTask.self, pool.FirstError()
}

type BlameConfigTask struct {
	context   *BlameConfigProcessorParams
	parent    *sdcpb.BlameTreeElement
	self      *sdcpb.BlameTreeElement
	selfEntry api.Entry
}

func NewBlameConfigTask(e api.Entry, c *BlameConfigProcessorParams) *BlameConfigTask {
	return &BlameConfigTask{
		context:   c,
		parent:    nil,
		self:      &sdcpb.BlameTreeElement{},
		selfEntry: e,
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

	// set KeyName for list element key levels
	if t.selfEntry.GetSchema() == nil {
		ancestorSchema, levelsUp := ops.GetFirstAncestorWithSchema(t.selfEntry)
		keys := ops.GetSchemaKeys(ancestorSchema)
		if len(keys) > 0 && levelsUp >= len(keys) {
			t.self.KeyName = keys[levelsUp-1]
		}
	}

	// process Value
	highestLe := t.selfEntry.GetLeafVariants().GetHighestPrecedence(false, true, true)
	if highestLe != nil {
		if highestLe.Update.Owner() != consts.DefaultsIntentName || t.context.IncludeDefaults {
			t.self.SetValue(highestLe.Update.Value()).SetOwner(highestLe.Update.Owner())

			// check if running equals the expected
			runningLe := t.selfEntry.GetLeafVariants().GetRunning()

			switch {
			case runningLe != nil:
				// if running value is different from the highest precedence value, then we have a deviation,
				// so we set the deviation value to the running value
				if !proto.Equal(runningLe.Update.Value(), highestLe.Update.Value()) {
					t.self.SetDeviationValue(runningLe.Value())
				}
			case runningLe == nil && highestLe.GetUpdate().Owner() != consts.DefaultsIntentName:
				// if running is nil and highest is not from default, then the deviation is from a non-existing running value,
				// so we set it to empty
				t.self.SetDeviationValue(&sdcpb.TypedValue{})
			}
		}
	}

	childs := t.selfEntry.GetChilds(types.DescendMethodActiveChilds)
	for _, childKey := range childs.SortedKeys() {
		childEntry := childs[childKey]
		childHighestLe := childEntry.GetLeafVariants().GetHighestPrecedence(false, true, true)
		if childHighestLe != nil {
			if childHighestLe.Update.Owner() == consts.DefaultsIntentName && !t.context.IncludeDefaults {
				continue
			}
		}

		child := &sdcpb.BlameTreeElement{Name: childEntry.PathName()}
		t.self.AddChild(child)

		// Create a new task for each child
		task := &BlameConfigTask{
			context:   t.context,
			parent:    t.self,
			self:      child,
			selfEntry: childEntry,
		}
		// Submit may fail if pool is closed or fail-fast error occurred
		if err := submit(task); err != nil {
			return err
		}
	}

	return nil
}
