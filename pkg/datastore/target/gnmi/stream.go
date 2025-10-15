package gnmi

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type StreamSync struct {
	ctx          context.Context
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	schemaClient dsutils.SchemaClientBound
}

func NewStreamSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound) *StreamSync {
	ctx, cancel := context.WithCancel(ctx)

	return &StreamSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
		schemaClient: schemaClient,
		ctx:          ctx,
	}
}

func (s *StreamSync) syncConfig() (*gnmi.SubscribeRequest, error) {

	opts := make([]gapi.GNMIOption, 0)
	subscriptionOpts := make([]gapi.GNMIOption, 0)
	for _, p := range s.config.Paths {
		subscriptionOpts = append(subscriptionOpts, gapi.Path(p))
	}
	switch s.config.Mode {
	case "sample":
		subscriptionOpts = append(subscriptionOpts, gapi.SubscriptionModeSAMPLE())
	case "on-change":
		subscriptionOpts = append(subscriptionOpts, gapi.SubscriptionModeON_CHANGE())
	}

	if s.config.Interval > 0 {
		subscriptionOpts = append(subscriptionOpts, gapi.SampleInterval(s.config.Interval))
	}
	opts = append(opts,
		gapi.EncodingCustom(utils.ParseGnmiEncoding(s.config.Encoding)),
		gapi.SubscriptionListModeSTREAM(),
		gapi.Subscription(subscriptionOpts...),
	)
	subReq, err := gapi.NewSubscribeRequest(opts...)
	if err != nil {
		return nil, err
	}
	return subReq, nil
}

func (s *StreamSync) Stop() error {
	s.cancel()
	return nil
}

func (s *StreamSync) Start() error {

	subReq, err := s.syncConfig()
	if err != nil {
		return err
	}

	log.Infof("sync %q: subRequest: %v", s.config.Name, subReq)

	respChan, errChan := s.target.Subscribe(s.ctx, subReq, s.config.Name)

	syncStartTime := time.Now()

	wpool := pool.NewWorkerPool[*gnmi.Notification](s.ctx, runtime.NumCPU())

	updChan := make(chan *NotificationData, 20)

	wpoolHandler := func(ctx context.Context, item *gnmi.Notification, submit func(*gnmi.Notification) error) error {
		sn := dsutils.ToSchemaNotification(item)
		// updates
		upds, err := treetypes.ExpandAndConvertIntent(s.ctx, s.schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, sn.GetUpdate(), item.GetTimestamp())
		if err != nil {
			log.Errorf("sync expanding error: %v", err)
		}

		deletes := sdcpb.NewPathSet()
		if len(item.GetDelete()) > 0 {
			for _, del := range item.GetDelete() {
				deletes.AddPath(dsutils.FromGNMIPath(item.GetPrefix(), del))
			}
		}

		updChan <- &NotificationData{
			updates: upds,
			deletes: deletes,
		}

		return nil
	}

	wpool.Start(wpoolHandler)

	syncToRunning := func(syncTree *tree.RootEntry, m *sync.Mutex, logCount bool) (*tree.RootEntry, error) {
		m.Lock()
		defer m.Unlock()

		startTime := time.Now()
		result, err := syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
		if err != nil {
			if errors.Is(err, tree.ErrorIntentNotPresent) {
				log.Info("sync no config changes")
				// all good no data present
				return syncTree, nil
			}
			log.Errorf("sync tree export error: %v", err)
			return s.runningStore.NewEmptyTree(s.ctx)
		}

		if logCount {
			log.Infof("Syncing: %d elements, %d deletes ", result.GetRoot().CountTerminals(), len(result.GetExplicitDeletes()))
		}

		log.Infof("TreeExport to proto took: %s", time.Since(startTime))
		startTime = time.Now()

		err = s.runningStore.ApplyToRunning(s.ctx, result)
		if err != nil {
			log.Errorf("sync import to running error: %v", err)
			return s.runningStore.NewEmptyTree(s.ctx)
		}
		log.Infof("Import to SyncTree took: %s", time.Since(startTime))
		return s.runningStore.NewEmptyTree(s.ctx)
	}

	syncResponse := make(chan struct{})

	go func() {
		if err != nil {
			log.Errorf("sync newemptytree error: %v", err)
			return
		}
		defer wpool.CloseForSubmit()
		for {
			select {
			case <-s.ctx.Done():
				return
			case err, ok := <-errChan:
				if !ok {
					return
				}
				if err != nil {
					log.Errorf("Error stream sync: %s", err)
					return
				}
			case resp, ok := <-respChan:
				if !ok {
					return
				}
				switch r := resp.GetResponse().(type) {
				case *gnmi.SubscribeResponse_Update:
					err := wpool.Submit(resp.GetUpdate())
					if err != nil {
						log.Errorf("error processing Notifications: %s", err)
						continue
					}
				case *gnmi.SubscribeResponse_SyncResponse:
					log.Info("SyncResponse flag received")
					log.Infof("Duration since sync Start: %s", time.Since(syncStartTime))
					syncResponse <- struct{}{}

				case *gnmi.SubscribeResponse_Error:
					log.Error(r.Error.Message)
				}
			}
		}
	}()

	go func(cUS <-chan *NotificationData, syncResponse <-chan struct{}) {

		syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
		if err != nil {
			log.Errorf("error creating new sync tree: %v", err)
			return
		}
		syncTreeMutex := &sync.Mutex{}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		// disable ticker until after the initial full sync is done
		tickerActive := false

		uif := treetypes.NewUpdateInsertFlags()

		for {
			select {
			case <-s.ctx.Done():
				return
			case noti, ok := <-cUS:
				if !ok {
					return
				}
				err := syncTree.AddUpdatesRecursive(s.ctx, noti.updates, uif)
				if err != nil {
					log.Errorf("error adding to sync tree: %v", err)
				}
				syncTree.AddExplicitDeletes(tree.RunningIntentName, tree.RunningValuesPrio, noti.deletes)
			case <-syncResponse:
				syncTree, err = syncToRunning(syncTree, syncTreeMutex, true)
				if err != nil {
					// TODO
					log.Errorf("syncToRunning Error %v", err)
				}
				tickerActive = true
			case <-ticker.C:
				if !tickerActive {
					log.Info("Skipping a sync tick - initial sync not finished yet")
					continue
				}
				log.Info("SyncRunning due to ticker")
				syncTree, err = syncToRunning(syncTree, syncTreeMutex, true)
				if err != nil {
					// TODO
					log.Errorf("syncToRunning Error %v", err)
				}
			}
		}
	}(updChan, syncResponse)

	return nil
}

type SyncTarget interface {
	Subscribe(ctx context.Context, req *gnmi.SubscribeRequest, subscriptionName string) (chan *gnmi.SubscribeResponse, chan error)
}

type NotificationData struct {
	updates treetypes.UpdateSlice
	deletes *sdcpb.PathSet
}
