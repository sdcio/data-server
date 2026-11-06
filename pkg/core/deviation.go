package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	// deviationBuffer is the fixed capacity of a deviation watch channel.
	deviationBuffer = 8
	// deviationStallWarn is how long a send may block before it is logged as a warning.
	deviationStallWarn = 30 * time.Second
)

var watcherSeq atomic.Uint64

// WatchDeviations returns a channel of deviation events for the named
// datastore. The channel closes when ctx is done, the datastore is deleted or
// the Core stops. The buffer is small and fixed: when it is full the datastore's
// deviation producer blocks until the watcher reads, so a stalled watcher
// delays deviation runs of that datastore and drops nothing. A send blocked for
// 30 seconds is logged as a warning. A START received while a run is still
// open is logged and ignored.
func (c *Core) WatchDeviations(ctx context.Context, name string) (<-chan api.DeviationEvent, error) {
	ds, err := c.getDatastore(name)
	if err != nil {
		return nil, err
	}
	w := &deviationWatcher{
		datastore: name,
		ctx:       ctx,
		coreCtx:   c.ctx,
		ch:        make(chan api.DeviationEvent, deviationBuffer),
		warnAfter: deviationStallWarn,
		log:       logf.FromContext(ctx).WithName("WatchDeviations").WithValues("datastore-name", name),
	}
	ds.AddDeviationClient(fmt.Sprintf("in-process-%d", watcherSeq.Add(1)), w)
	go func() {
		select {
		case <-ctx.Done():
		case <-c.ctx.Done():
		case <-ds.Done():
		}
		ds.StopDeviationsWatch(w)
		w.close()
	}()
	return w.ch, nil
}

// deviationWatcher adapts the datastore's sdcpb deviation stream to a channel
// of domain events.
type deviationWatcher struct {
	datastore string
	ctx       context.Context
	coreCtx   context.Context
	warnAfter time.Duration
	log       logr.Logger

	// mu serialises Send and close so the channel is never closed mid-send.
	mu     sync.Mutex
	ch     chan api.DeviationEvent
	closed bool
	inRun  bool
}

func (w *deviationWatcher) Context() context.Context { return w.ctx }

func (w *deviationWatcher) Send(rsp *sdcpb.WatchDeviationResponse) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return context.Canceled
	}
	ev := toDeviationEvent(rsp)
	switch ev.Kind {
	case api.DeviationStart:
		if w.inRun {
			w.log.Info("duplicate deviation START ignored")
			return nil
		}
		w.inRun = true
	case api.DeviationEnd:
		w.inRun = false
	}
	return w.deliver(ev)
}

// deliver blocks until the event is buffered or the watch ends.
func (w *deviationWatcher) deliver(ev api.DeviationEvent) error {
	select {
	case w.ch <- ev:
		return nil
	default:
	}
	w.log.V(logf.VDebug).Info("deviation watcher is slow, send blocked")
	timer := time.NewTimer(w.warnAfter)
	defer timer.Stop()
	for {
		select {
		case w.ch <- ev:
			return nil
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-w.coreCtx.Done():
			return w.coreCtx.Err()
		case <-timer.C:
			w.log.Error(nil, "deviation watcher has not consumed an event, deviation producer is blocked", "severity", "WARN", "blocked-for", w.warnAfter.String())
		}
	}
}

func (w *deviationWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.ch)
	}
}

func deviationReason(r sdcpb.DeviationReason) api.DeviationReason {
	switch r {
	case sdcpb.DeviationReason_UNHANDLED:
		return api.DeviationReasonUnhandled
	case sdcpb.DeviationReason_NOT_APPLIED:
		return api.DeviationReasonNotApplied
	case sdcpb.DeviationReason_OVERRULED:
		return api.DeviationReasonOverruled
	case sdcpb.DeviationReason_INTENT_EXISTS:
		return api.DeviationReasonIntentExists
	default:
		return api.DeviationReasonUnknown
	}
}

func toDeviationEvent(rsp *sdcpb.WatchDeviationResponse) api.DeviationEvent {
	ev := api.DeviationEvent{
		Datastore: rsp.GetName(),
		Intent:    rsp.GetIntent(),
		Reason:    deviationReason(rsp.GetReason()),
	}
	switch rsp.GetEvent() {
	case sdcpb.DeviationEvent_START:
		ev.Kind = api.DeviationStart
	case sdcpb.DeviationEvent_END:
		ev.Kind = api.DeviationEnd
	case sdcpb.DeviationEvent_CLEAR:
		ev.Kind = api.DeviationClear
	default:
		ev.Kind = api.DeviationUpdate
		ev.Entries = []api.DeviationEntry{{
			Path:     rsp.GetPath(),
			Expected: rsp.GetExpectedValue(),
			Current:  rsp.GetCurrentValue(),
		}}
	}
	return ev
}
