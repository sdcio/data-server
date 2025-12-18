package datastore

import (
	"context"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
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

	log := logf.FromContext(d.ctx)
	log.Info("new deviation client", "client", pName)
	return nil
}

func (d *Datastore) StopDeviationsWatch(peer string) {
	log := logf.FromContext(d.ctx)
	log.Info("deviation client removed", "peer", peer)
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.deviationClients, peer)
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
			d.m.RLock()
			deviationClientNames := make([]string, 0, len(d.deviationClients))
			deviationClients := map[string]sdcpb.DataServer_WatchDeviationsServer{}
			for clientIdentifier, devStream := range d.deviationClients {
				deviationClients[clientIdentifier] = devStream
				if devStream.Context().Err() != nil {
					log.V(logf.VWarn).Error(devStream.Context().Err(), "removing deviation client", "client", clientIdentifier)
					delete(deviationClients, clientIdentifier)
					continue
				}
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
			start := time.Now()
			deviationChan, err := d.calculateDeviations()
			if err != nil {
				log.Error(err, "failed to calculate deviations")
				continue
			}
			log.V(logf.VDebug).Info("calculated deviations", "duration", time.Since(start))
			d.SendDeviations(ctx, deviationChan, deviationClients)
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
	vPool := d.taskPool.NewVirtualPool(pool.VirtualTolerant, 1)
	for de := range ch {
		vPool.SubmitFunc(func(ctx context.Context, _ func(pool.Task) error) error {
			for clientIdentifier, dc := range deviationClients {
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
			return nil
		})
	}
	vPool.CloseForSubmit()
	log.Info("waiting for tasks in pool to finish")
	vPool.Wait()
	log.Info("pool finished")
}

type DeviationEntry interface {
	IntentName() string
	Reason() treetypes.DeviationReason
	Path() *sdcpb.Path
	CurrentValue() *sdcpb.TypedValue
	ExpectedValue() *sdcpb.TypedValue
}

func (d *Datastore) calculateDeviations() (<-chan *treetypes.DeviationEntry, error) {
	deviationChan := make(chan *treetypes.DeviationEntry, 10)

	d.syncTreeMutex.RLock()
	deviationTree, err := d.syncTree.DeepCopy(d.ctx)
	d.syncTreeMutex.RUnlock()
	if err != nil {
		return nil, err
	}

	addedIntentNames, err := d.LoadAllButRunningIntents(d.ctx, deviationTree, true)
	if err != nil {
		return nil, err
	}

	// Send IntentExists
	for _, n := range addedIntentNames {
		deviationChan <- treetypes.NewDeviationEntry(n, treetypes.DeviationReasonIntentExists, nil)
	}

	err = deviationTree.FinishInsertionPhase(d.ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		deviationTree.GetDeviations(d.ctx, deviationChan)
		close(deviationChan)
	}()

	return deviationChan, nil
}
