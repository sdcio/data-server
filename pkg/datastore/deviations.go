package datastore

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/pool"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
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
	log.Debugf("deviation watcher %s removed", peer)
}

func (d *Datastore) DeviationMgr(ctx context.Context, c *config.DeviationConfig) {
	log.Infof("%s: starting deviationMgr...", d.Name())
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
				log.Debugf("no deviation clients present %s", d.config.Name)
				continue
			}
			log.Debugf("deviations clients for %s: [ %s ]", d.config.Name, strings.Join(deviationClientNames, ", "))
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_START,
				})
				if err != nil {
					log.Errorf("error sending deviation to %s: %v", clientIdentifier, err)
				}
			}
			deviationChan, err := d.calculateDeviations(ctx)
			if err != nil {
				log.Error(err)
				continue
			}
			d.SendDeviations(deviationChan, deviationClients)
			for clientIdentifier, dc := range deviationClients {
				err := dc.Send(&sdcpb.WatchDeviationResponse{
					Name:  d.config.Name,
					Event: sdcpb.DeviationEvent_END,
				})
				if err != nil {
					log.Errorf("error sending deviation to %s: %v", clientIdentifier, err)
				}
			}
		}
	}
}

func (d *Datastore) SendDeviations(ch <-chan *treetypes.DeviationEntry, deviationClients map[string]sdcpb.DataServer_WatchDeviationsServer) {
	wg := &sync.WaitGroup{}
	vPool := d.taskPool.NewVirtualPool(pool.VirtualTolerant, 1)

	for de := range ch {
		wg.Add(1)
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
					log.Errorf("error sending deviation to %s: %v", clientIdentifier, err)
				}
			}
			wg.Done()
			return nil
		})
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
		deviationTree.GetDeviations(deviationChan)
		close(deviationChan)
	}()

	return deviationChan, nil
}
