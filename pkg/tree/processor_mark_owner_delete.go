package tree

import (
	"context"
	"sync"

	"github.com/sdcio/data-server/pkg/pool"
)

type MarkOwnerDeleteProcessor struct {
	config  *OwnerDeleteMarkerTaskConfig
	matches *Collector[*LeafEntry]
}

func NewOwnerDeleteMarker(c *OwnerDeleteMarkerTaskConfig) *MarkOwnerDeleteProcessor {
	return &MarkOwnerDeleteProcessor{
		config:  c,
		matches: NewCollector[*LeafEntry](20),
	}
}

func (o *MarkOwnerDeleteProcessor) Run(e Entry, pool pool.VirtualPoolI) error {

	err := pool.Submit(newOwnerDeleteMarkerTask(o.config, e, o.matches))
	if err != nil {
		return err
	}
	// close pool for additional external submission
	pool.CloseForSubmit()
	// wait for the pool to run dry
	pool.Wait()

	return pool.FirstError()
}

type OwnerDeleteMarkerTaskConfig struct {
	owner        string
	onlyIntended bool
}

func NewOwnerDeleteMarkerTaskConfig(owner string, onlyIntended bool) *OwnerDeleteMarkerTaskConfig {
	return &OwnerDeleteMarkerTaskConfig{
		owner:        owner,
		onlyIntended: onlyIntended,
	}
}

type ownerDeleteMarkerTask struct {
	config  *OwnerDeleteMarkerTaskConfig
	matches *Collector[*LeafEntry]
	e       Entry
}

func newOwnerDeleteMarkerTask(c *OwnerDeleteMarkerTaskConfig, e Entry, matches *Collector[*LeafEntry]) *ownerDeleteMarkerTask {
	return &ownerDeleteMarkerTask{
		config:  c,
		e:       e,
		matches: matches,
	}
}

func (x ownerDeleteMarkerTask) Run(ctx context.Context, submit func(pool.Task) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	le := x.e.GetLeafVariantEntries().MarkOwnerForDeletion(x.config.owner, x.config.onlyIntended)
	if le != nil {
		x.matches.Append(le)
	}
	for _, c := range x.e.GetChilds(DescendMethodAll) {
		submit(newOwnerDeleteMarkerTask(x.config, c, x.matches))
	}
	return nil
}

// Collector is a concurrent-safe, append-only collector for values of type T.
type Collector[T any] struct {
	mu  sync.Mutex
	out []T
}

// NewCollector creates a Collector with a preallocated capacity.
// Pass 0 if you don't want to preallocate.
func NewCollector[T any](cap int) *Collector[T] {
	if cap < 0 {
		cap = 0
	}
	return &Collector[T]{out: make([]T, 0, cap)}
}

// Append appends one element to the collector.
func (c *Collector[T]) Append(x T) {
	c.mu.Lock()
	c.out = append(c.out, x)
	c.mu.Unlock()
}

// AppendAll appends all elements from the provided slice.
// This is slightly more efficient than calling Append in a loop.
func (c *Collector[T]) AppendAll(xs []T) {
	if len(xs) == 0 {
		return
	}
	c.mu.Lock()
	c.out = append(c.out, xs...)
	c.mu.Unlock()
}

// Len returns the current number of elements.
func (c *Collector[T]) Len() int {
	c.mu.Lock()
	n := len(c.out)
	c.mu.Unlock()
	return n
}
