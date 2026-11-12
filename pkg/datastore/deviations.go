package datastore

import (
	"context"
	"errors"
	"time"

	pubapi "github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// DeviationSink receives the deviation events of one watcher. Send may block;
// the deviation producer of the datastore waits for it. Context ends the watch.
type DeviationSink interface {
	Context() context.Context
	Send(pubapi.DeviationEvent) error
}

// AddDeviationClient registers sink under the identifier client, which is used
// for logging and must be unique among the datastore's sinks.
func (d *Datastore) AddDeviationClient(client string, sink DeviationSink) {
	log := logf.FromContext(d.ctx)
	d.m.Lock()
	defer d.m.Unlock()
	d.deviationClients[sink] = client

	log.Info("new deviation client", "client", client)
}

// StopDeviationsWatch unregisters sink.
func (d *Datastore) StopDeviationsWatch(sink DeviationSink) {
	log := logf.FromContext(d.ctx)
	d.m.Lock()
	defer d.m.Unlock()
	client := d.deviationClients[sink]
	delete(d.deviationClients, sink)
	log.Info("deviation client removed", "peer", client)
}

// Done is closed when the datastore is stopped.
func (d *Datastore) Done() <-chan struct{} {
	return d.ctx.Done()
}

func (d *Datastore) DeviationMgr(ctx context.Context, c *config.DeviationConfig) {
	log := logf.FromContext(ctx).WithName("DeviationManager").WithValues("target-name", d.config.Name)
	ctx = logf.IntoContext(ctx, log)

	log.Info("starting deviation manager")
	ticker := time.NewTicker(c.Interval)
	defer func() {
		ticker.Stop()
		log.Info("deviation manager stopped")
	}()
	for {
		select {
		case <-ctx.Done():
			log.Info("datastore context done, stopping deviation manager")
			return
		case <-ticker.C:
			log.V(logf.VDebug).Info("deviation calc run - start")

			deviationClients := map[string]DeviationSink{}
			deviationClientNames := make([]string, 0, len(d.deviationClients))

			// encap in func to use defer for the lock
			func() {
				d.m.RLock()
				defer d.m.RUnlock()
				for devStream, peerIdentifier := range d.deviationClients {
					deviationClients[peerIdentifier] = devStream
					if devStream.Context().Err() != nil {
						log.Error(devStream.Context().Err(), "deviation client context error", "severity", "WARN", "client", peerIdentifier)
						continue
					}
					deviationClientNames = append(deviationClientNames, peerIdentifier)
				}
			}()
			if len(deviationClients) == 0 {
				log.V(logf.VDebug).Info("no deviation clients present")
				continue
			}
			log.V(logf.VDebug).Info("deviation clients", "clients", deviationClientNames)
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(pubapi.DeviationEvent{
					Datastore: d.config.Name,
					Kind:      pubapi.DeviationStart,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
			start := time.Now()
			deviationChan, err := d.calculateDeviations(ctx)
			if err != nil {
				log.Error(err, "failed to calculate deviations")
				continue
			}
			log.V(logf.VDebug).Info("calculate deviations", "duration", time.Since(start))
			d.SendDeviations(ctx, deviationChan, deviationClients)
			log.Info("Before sending DeviationEvent_END")
			for clientIdentifier, dc := range deviationClients {
				if dc.Context().Err() != nil {
					continue
				}
				err := dc.Send(pubapi.DeviationEvent{
					Datastore: d.config.Name,
					Kind:      pubapi.DeviationEnd,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
			log.V(logf.VDebug).Info("deviation calc run - finished")
		}
	}
}

// DeviationsStats intent name -> reason -> count
type DeviationsStats map[string]map[treetypes.DeviationReason]int

func (d DeviationsStats) Add(deviation *treetypes.DeviationEntry) {
	name := deviation.IntentName()
	if d[name] == nil {
		d[name] = make(map[treetypes.DeviationReason]int)
	}
	d[name][deviation.Reason()]++
}

func (d DeviationsStats) LogValue() map[string]map[string]int {
	stats := make(map[string]map[string]int, len(d))
	for intentName, reasonCounts := range d {
		stats[intentName] = make(map[string]int, len(reasonCounts))
		for reason, count := range reasonCounts {
			stats[intentName][reason.String()] = count
		}
	}
	return stats
}

func deviationReason(r treetypes.DeviationReason) pubapi.DeviationReason {
	switch r {
	case treetypes.DeviationReasonUnhandled:
		return pubapi.DeviationReasonUnhandled
	case treetypes.DeviationReasonNotApplied:
		return pubapi.DeviationReasonNotApplied
	case treetypes.DeviationReasonOverruled:
		return pubapi.DeviationReasonOverruled
	case treetypes.DeviationReasonIntentExists:
		return pubapi.DeviationReasonIntentExists
	default:
		return pubapi.DeviationReasonUnknown
	}
}

func (d *Datastore) SendDeviations(ctx context.Context, ch <-chan *treetypes.DeviationEntry, deviationClients map[string]DeviationSink) {
	log := logf.FromContext(ctx)
	deviationsStats := make(DeviationsStats)
	for deviation := range ch {
		deviationsStats.Add(deviation)
		for clientIdentifier, dc := range deviationClients {
			if dc.Context().Err() != nil {
				continue
			}

			err := dc.Send(pubapi.DeviationEvent{
				Datastore: d.config.Name,
				Kind:      pubapi.DeviationUpdate,
				Intent:    deviation.IntentName(),
				Reason:    deviationReason(deviation.Reason()),
				Path:      deviation.Path(),
				Expected:  deviation.ExpectedValue(),
				Current:   deviation.CurrentValue(),
			})
			if err != nil {
				// ignore client-side cancellation (context closed) as it's expected when a client disconnects
				if dc.Context().Err() != nil || errors.Is(err, context.Canceled) {
					log.V(logf.VDebug).Info("client context closed, skipping send", "client-identifier", clientIdentifier, "err", err)
					continue
				}
				log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
			}
		}
	}
	log.Info("deviation stats", "stats", deviationsStats.LogValue())
}

type DeviationEntry interface {
	IntentName() string
	Reason() treetypes.DeviationReason
	Path() *sdcpb.Path
	CurrentValue() *sdcpb.TypedValue
	ExpectedValue() *sdcpb.TypedValue
}

func (d *Datastore) calculateDeviations(ctx context.Context) (<-chan *treetypes.DeviationEntry, error) {

	log := logf.FromContext(ctx)

	d.syncTreeMutex.RLock()
	deviationTree, err := d.syncTree.DeepCopy(ctx)
	d.syncTreeMutex.RUnlock()
	if err != nil {
		return nil, err
	}

	addedIntentNames, err := d.LoadAllButRunningIntents(ctx, deviationTree)
	if err != nil {
		return nil, err
	}

	err = deviationTree.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	if log := log.V(logf.VTrace); log.Enabled() {
		log.Info("deviation tree", "content", deviationTree.String())
		log.Info("nonrevertive infos", "data", deviationTree.GetTreeContext().NonRevertiveInfo().String())
	}

	deviationChan := make(chan *treetypes.DeviationEntry, 10)
	go func() {
		defer close(deviationChan)
		// Send IntentExists
		for _, n := range addedIntentNames {
			deviationChan <- treetypes.NewDeviationEntry(n, treetypes.DeviationReasonIntentExists, nil)
		}

		err := ops.GetDeviations(ctx, deviationTree.Entry, &ops.GetDeviationParams{Ch: deviationChan}, d.taskPool)
		if err != nil {
			log.Error(err, "failed to run deviation processor")
		}
	}()

	return deviationChan, nil
}
