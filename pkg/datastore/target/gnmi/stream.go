package gnmi

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type StreamSync struct {
	ctx          context.Context
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	schemaClient dsutils.SchemaClientBound
	vpoolFactory pool.VirtualPoolFactory
}

func NewStreamSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound, vpoolFactory pool.VirtualPoolFactory) *StreamSync {
	ctx, cancel := context.WithCancel(ctx)

	// add the sync name to the logger values
	log := logger.FromContext(ctx).WithValues("sync", c.Name)
	ctx = logger.IntoContext(ctx, log)

	return &StreamSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
		schemaClient: schemaClient,
		ctx:          ctx,
		vpoolFactory: vpoolFactory,
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
		gapi.Extension(&gnmi_ext.Extension{
			Ext: &gnmi_ext.Extension_ConfigSubscription{
				ConfigSubscription: &gnmi_ext.ConfigSubscription{
					Action: &gnmi_ext.ConfigSubscription_Start{
						Start: &gnmi_ext.ConfigSubscriptionStart{},
					},
				},
			},
		}),
	)
	subReq, err := gapi.NewSubscribeRequest(opts...)
	if err != nil {
		return nil, err
	}
	return subReq, nil
}

func (s *StreamSync) Stop() error {
	log := logger.FromContext(s.ctx)
	log.Info("Stopping Sync")
	s.cancel()
	return nil
}

func (s *StreamSync) Name() string {
	return s.config.Name
}

func (s *StreamSync) Start() error {
	log := logger.FromContext(s.ctx)
	log.Info("Starting Sync")

	updChan := make(chan *NotificationData, 20)

	syncResponse := make(chan struct{})

	subReq, err := s.syncConfig()
	if err != nil {
		return err
	}

	// start the gnmi subscribe request, that also used the pool for
	go s.gnmiSubscribe(subReq, updChan, syncResponse)
	//
	go s.buildTreeSyncWithDatastore(updChan, syncResponse)

	return nil
}

func (s *StreamSync) buildTreeSyncWithDatastore(cUS <-chan *NotificationData, syncResponse <-chan struct{}) {
	log := logger.FromContext(s.ctx)
	syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Error(err, "failure creating new sync tree")
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
			log.V(logger.VDebug).Info("stopping sync due to context done")
			return
		case noti, ok := <-cUS:
			if !ok {
				return
			}
			err := syncTree.AddUpdatesRecursive(s.ctx, noti.updates, uif)
			if err != nil {
				log.Error(err, "failed adding update to synctree")
			}
			syncTree.GetTreeContext().AddExplicitDeletes(tree.RunningIntentName, tree.RunningValuesPrio, noti.deletes)
		case <-syncResponse:
			syncTree, err = s.syncToRunning(syncTree, syncTreeMutex, true)
			tickerActive = true
			if err != nil {
				log.Error(err, "failed committing synctree to running")
			}
		case <-ticker.C:
			if !tickerActive {
				log.Info("Skipping a sync tick - initial sync not finished yet")
				continue
			}
			log.Info("SyncRunning due to ticker")
			syncTree, err = s.syncToRunning(syncTree, syncTreeMutex, true)
			if err != nil {
				log.Error(err, "failed committing synctree to running")
			}
		}
	}
}

func (s *StreamSync) gnmiSubscribe(subReq *gnmi.SubscribeRequest, updChan chan<- *NotificationData, syncResponse chan<- struct{}) {
	var err error
	log := logger.FromContext(s.ctx)
	log.V(logger.VTrace).Info("starting gnmi subscription", "subscripton", subReq)

	respChan, errChan := s.target.Subscribe(s.ctx, subReq, s.config.Name)

	taskPool := s.vpoolFactory.NewVirtualPool(pool.VirtualTolerant)
	defer taskPool.CloseForSubmit()
	taskParams := NewNotificationProcessorTaskParameters(updChan, s.schemaClient)

	syncStartTime := time.Now()
	for {
		select {
		case <-s.ctx.Done():
			return
		case err, ok := <-errChan:
			if !ok {
				return
			}
			if err != nil {
				log.Error(err, "failed stream sync")
				return
			}
		case resp, ok := <-respChan:
			if !ok {
				return
			}
			switch r := resp.GetResponse().(type) {
			case *gnmi.SubscribeResponse_Update:
				err = taskPool.Submit(newNotificationProcessorTask(resp.GetUpdate(), taskParams))
				if err != nil {
					log.Error(err, "failure processing notifications")
					continue
				}
			case *gnmi.SubscribeResponse_SyncResponse:
				log.Info("SyncResponse flag received", "initial sync duration", time.Since(syncStartTime).String())
				syncResponse <- struct{}{}

			case *gnmi.SubscribeResponse_Error:
				log.Error(nil, "gnmi subscription error", "error", r.Error.Message)
			}
		}
	}
}

func (s *StreamSync) syncToRunning(syncTree *tree.RootEntry, m *sync.Mutex, logCount bool) (*tree.RootEntry, error) {
	log := logger.FromContext(s.ctx)
	m.Lock()
	defer m.Unlock()

	startTime := time.Now()
	result, err := syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio)
	log.V(logger.VTrace).Info("exported tree", "tree", result.String())

	if err != nil {
		if errors.Is(err, tree.ErrorIntentNotPresent) {
			log.Info("sync no config changes")
			// all good no data present
			return syncTree, nil
		}
		log.Error(err, "sync tree export error: %v")
		return s.runningStore.NewEmptyTree(s.ctx)
	}
	// extract the explicit deletes
	deletes := result.ExplicitDeletes
	// set them to nil
	result.ExplicitDeletes = nil
	if logCount {
		log.V(logger.VDebug).Info("syncing to running", "elements", result.GetRoot().CountTerminals(), "deletes", len(result.GetExplicitDeletes()))
	}

	log.V(logger.VTrace).Info("synctree export done", "duration", time.Since(startTime).String())
	startTime = time.Now()

	err = s.runningStore.ApplyToRunning(s.ctx, deletes, proto.NewProtoTreeImporter(result))
	if err != nil {
		log.Error(err, "failed importing sync to running")
		return s.runningStore.NewEmptyTree(s.ctx)
	}
	log.V(logger.VTrace).Info("import to running tree done", "duration", time.Since(startTime).String())
	return s.runningStore.NewEmptyTree(s.ctx)
}

type SyncTarget interface {
	Subscribe(ctx context.Context, req *gnmi.SubscribeRequest, subscriptionName string) (chan *gnmi.SubscribeResponse, chan error)
}

type NotificationData struct {
	updates []*treetypes.PathAndUpdate
	deletes *sdcpb.PathSet
}

type notificationProcessorTask struct {
	item   *gnmi.Notification
	params *NotificationProcessorTaskParameters
}

type NotificationProcessorTaskParameters struct {
	notificationResult chan<- *NotificationData
	schemaClientBound  dsutils.SchemaClientBound
}

func NewNotificationProcessorTaskParameters(notificationResult chan<- *NotificationData, scb dsutils.SchemaClientBound) *NotificationProcessorTaskParameters {
	return &NotificationProcessorTaskParameters{
		notificationResult: notificationResult,
		schemaClientBound:  scb,
	}
}

func newNotificationProcessorTask(item *gnmi.Notification, params *NotificationProcessorTaskParameters) *notificationProcessorTask {
	return &notificationProcessorTask{
		item:   item,
		params: params,
	}
}

func (t *notificationProcessorTask) Run(ctx context.Context, _ func(pool.Task) error) error {
	log := logger.FromContext(ctx)
	sn := dsutils.ToSchemaNotification(ctx, t.item)
	// updates
	upds, err := treetypes.ExpandAndConvertIntent(ctx, t.params.schemaClientBound, tree.RunningIntentName, tree.RunningValuesPrio, sn.GetUpdate(), t.item.GetTimestamp())
	if err != nil {
		log.Error(err, "expansion and conversion failed")
	}

	deletes := sdcpb.NewPathSet()
	if len(t.item.GetDelete()) > 0 {
		for _, del := range t.item.GetDelete() {
			deletes.AddPath(dsutils.FromGNMIPath(t.item.GetPrefix(), del))
		}
	}

	t.params.notificationResult <- &NotificationData{
		updates: upds,
		deletes: deletes,
	}

	return nil
}
