package datastore

import (
	"context"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	status "google.golang.org/grpc/status"
)

func (d *Datastore) WatchDeviations(req *sdcpb.WatchDeviationRequest, stream sdcpb.DataServer_WatchDeviationsServer) error {
	d.m.Lock()
	defer d.m.Unlock()

	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "missing peer info")
	}
	pName := p.Addr.String()

	d.deviationClients[pName] = stream
	return nil
}

func (d *Datastore) StopDeviationsWatch(peer string) {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.deviationClients, peer)
	logf.DefaultLogger.V(logf.VDebug).Info("deviation watcher removed", "peer", peer)
}

func (d *Datastore) DeviationMgr(ctx context.Context, c *config.DeviationConfig) {
	log := logf.FromContext(ctx)
	log = log.WithValues("target-name", d.config.Name)
	ctx = logf.IntoContext(ctx, log)

	log.Info("starting deviation manager")
	ticker := time.NewTicker(c.Interval)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.m.RLock()
			deviationClientNames := make([]string, 0, len(d.deviationClients))
			deviationClients := map[string]sdcpb.DataServer_WatchDeviationsServer{}
			for clientIdentifier, devStream := range d.deviationClients {
				deviationClients[clientIdentifier] = devStream
				deviationClientNames = append(deviationClientNames, clientIdentifier)
			}
			d.m.RUnlock()
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
			deviationChan, err := d.calculateDeviations(ctx)
			if err != nil {
				log.Error(err, "failed to calculate deviations")
				continue
			}
			d.SendDeviations(ctx, deviationChan, deviationClients)
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_END,
				})
				if err != nil {
					log.Error(err, "error sending deviation", "client-identifier", clientIdentifier)
				}
			}
		}
	}
}

func (d *Datastore) SendDeviations(ctx context.Context, ch <-chan *treetypes.DeviationEntry, deviationClients map[string]sdcpb.DataServer_WatchDeviationsServer) {
	log := logf.FromContext(ctx)
	wg := &sync.WaitGroup{}
	for de := range ch {
		wg.Add(1)
		go func(de DeviationEntry, dcs map[string]sdcpb.DataServer_WatchDeviationsServer) {
			for clientIdentifier, dc := range dcs {
				// skip deviation clients with closed context
				if dc.Context().Err() != nil {
					continue
				}

				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:          d.config.Name,
					Intent:        de.IntentName(),
					Event:         sdcpb.DeviationEvent_UPDATE,
					Reason:        sdcpb.DeviationReason(de.Reason()),
					Path:          de.Path(),
					ExpectedValue: de.ExpectedValue(),
					CurrentValue:  de.CurrentValue(),
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
			wg.Done()
		}(de, deviationClients)
	}
	wg.Wait()
}

type DeviationEntry interface {
	IntentName() string
	Reason() treetypes.DeviationReason
	Path() *sdcpb.Path
	CurrentValue() *sdcpb.TypedValue
	ExpectedValue() *sdcpb.TypedValue
}

func (d *Datastore) calculateDeviations(ctx context.Context) (<-chan *treetypes.DeviationEntry, error) {
	deviationChan := make(chan *treetypes.DeviationEntry, 10)

	d.syncTreeMutex.RLock()
	deviationTree, err := d.syncTree.DeepCopy(ctx)
	if err != nil {
		return nil, err
	}
	d.syncTreeMutex.RUnlock()

	addedIntentNames, err := d.LoadAllButRunningIntents(ctx, deviationTree, true)
	if err != nil {
		return nil, err
	}

	// Send IntentExists
	for _, n := range addedIntentNames {
		deviationChan <- treetypes.NewDeviationEntry(n, treetypes.DeviationReasonIntentExists, nil)
	}

	err = deviationTree.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		deviationTree.GetDeviations(ctx, deviationChan)
		close(deviationChan)
	}()

	return deviationChan, nil
}
