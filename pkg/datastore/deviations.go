package datastore

import (
	"context"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/logger"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	status "google.golang.org/grpc/status"
)

func (d *Datastore) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	log := logf.FromContext(d.ctx)

	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	}
	peerAddr := p.Addr.String()

	d.m.Lock()
	defer d.m.Unlock()
	d.deviationClients[stream] = peerAddr

	log.Info("new deviation client", "client", peerAddr)
	return nil
}

func (d *Datastore) StopDeviationsWatch(stream sdcpb.DataServer_WatchDeviationsServer) {
	log := logf.FromContext(d.ctx)
	d.m.Lock()
	defer d.m.Unlock()
	peer := d.deviationClients[stream]
	delete(d.deviationClients, stream)
	log.Info("deviation client removed", "peer", peer)
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

			deviationClients := map[string]sdcpb.DataServer_WatchDeviationsServer{}
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
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_START,
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
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_END,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
			log.V(logf.VDebug).Info("deviation calc run - finished")
		}
	}
}

func (d *Datastore) SendDeviations(ctx context.Context, ch <-chan *treetypes.DeviationEntry, deviationClients map[string]sdcpb.DataServer_WatchDeviationsServer) {
	log := logf.FromContext(ctx)
	for deviation := range ch {
		for clientIdentifier, dc := range deviationClients {
			if dc.Context().Err() != nil {
				continue
			}

			if log := log.V(logger.VTrace); log.Enabled() {
				if deviation.Reason() == treetypes.DeviationReasonNotApplied { // TODO add check for trace level Trace
					current := "nil"
					if deviation.CurrentValue() != nil {
						current = deviation.CurrentValue().ToString()
					}
					log.Info("NOT APPLIED", "path", deviation.Path().ToXPath(false), "actual value", current, "expected value", deviation.ExpectedValue().ToString(), "intent", deviation.IntentName())
				}
			}

			err := dc.Send(&sdcpb.WatchDeviationResponse{
				Name:          d.config.Name,
				Intent:        deviation.IntentName(),
				Event:         sdcpb.DeviationEvent_UPDATE,
				Reason:        sdcpb.DeviationReason(deviation.Reason()),
				Path:          deviation.Path(),
				ExpectedValue: deviation.ExpectedValue(),
				CurrentValue:  deviation.CurrentValue(),
			})
			if err != nil {
				// ignore client-side cancellation (context closed) as it's expected when a client disconnects
				if dc.Context().Err() != nil || status.Code(err) == codes.Canceled {
					log.V(logf.VDebug).Info("client context closed, skipping send", "client-identifier", clientIdentifier, "err", err)
					continue
				}
				log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
			}
		}
	}
}

type DeviationEntry interface {
	IntentName() string
	Reason() treetypes.DeviationReason
	Path() *sdcpb.Path
	CurrentValue() *sdcpb.TypedValue
	ExpectedValue() *sdcpb.TypedValue
}

func (d *Datastore) calculateDeviations(ctx context.Context) (<-chan *treetypes.DeviationEntry, error) {

	log := logger.FromContext(ctx)

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

	if log := log.V(logger.VTrace); log.Enabled() {
		log.Info("deviation tree", "content", deviationTree.String())
	}

	deviationChan := make(chan *treetypes.DeviationEntry, 10)
	go func() {
		defer close(deviationChan)
		// Send IntentExists
		for _, n := range addedIntentNames {
			deviationChan <- treetypes.NewDeviationEntry(n, treetypes.DeviationReasonIntentExists, nil)
		}

		deviationTree.GetDeviations(ctx, deviationChan)
	}()

	return deviationChan, nil
}
