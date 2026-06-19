package gnmi

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	syncSignalBufferSize         = 1
	defaultUpdChanBufSize        = 20
	defaultNotifSendTimeout      = 5 * time.Second
	notificationSlowSendWarnTime = 500 * time.Millisecond
)

type StreamSync struct {
	ctx          context.Context
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	schemaClient dsutils.SchemaClientBound
	vpoolFactory pool.VirtualPoolFactory

	// updChanBufSize controls the notification channel buffer. Default is
	// defaultUpdChanBufSize; tests may override before calling Start.
	updChanBufSize int
	// notifSendTimeout is the per-notification delivery deadline. Default is
	// defaultNotifSendTimeout; tests may override before calling Start.
	notifSendTimeout time.Duration
}

func NewStreamSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound, vpoolFactory pool.VirtualPoolFactory) *StreamSync {
	ctx, cancel := context.WithCancel(ctx)

	// add the sync name to the logger values
	log := logger.FromContext(ctx).WithValues("sync", c.Name)
	ctx = logger.IntoContext(ctx, log)

	return &StreamSync{
		config:           c,
		target:           target,
		cancel:           cancel,
		runningStore:     runningStore,
		schemaClient:     schemaClient,
		ctx:              ctx,
		vpoolFactory:     vpoolFactory,
		updChanBufSize:   defaultUpdChanBufSize,
		notifSendTimeout: defaultNotifSendTimeout,
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

	updChan := make(chan *NotificationData, s.updChanBufSize)

	// Keep a single pending sync signal to avoid blocking the subscribe loop.
	syncResponse := make(chan struct{}, syncSignalBufferSize)

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

// buildTreeSyncWithDatastore accumulates incoming notifications into a local
// syncTree and commits it to Running on SyncResponse and every ticker tick.
//
// The commit is handed off to a background goroutine so the main select loop
// never blocks on ApplyToRunning. This prevents notifications from timing out
// and being silently dropped while a slow performRevert holds the goroutine
// (see GitHub issue #440).
func (s *StreamSync) buildTreeSyncWithDatastore(cUS <-chan *NotificationData, syncResponse <-chan struct{}) {
	log := logger.FromContext(s.ctx)
	syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Error(err, "failure creating new sync tree")
		return
	}

	uif := treetypes.NewUpdateInsertFlags()

	// tickerChan starts as nil (receives from a nil channel block forever in
	// Go, so the ticker case never fires). It is set to a real ticker after
	// the initial SyncResponse so the ticker only runs post-initial-sync.
	var tickerChan <-chan time.Time

	// syncRunning is true while a syncToRunning goroutine is in flight.
	// The ticker case skips when true; the syncResponse case always proceeds.
	var syncRunning atomic.Bool

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
			syncTree.GetTreeContext().OperationState().ExplicitDeletes().Add(consts.RunningIntentName, consts.RunningValuesPrio, noti.deletes)
		case <-syncResponse:
			treeToCommit := syncTree
			syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
			if err != nil {
				log.Error(err, "failure creating new sync tree after sync response")
				return
			}
			if tickerChan == nil {
				t := time.NewTicker(5 * time.Second)
				defer t.Stop()
				tickerChan = t.C
			}
			syncRunning.Store(true)
			go func() {
				defer syncRunning.Store(false)
				if err := s.syncToRunning(treeToCommit, true); err != nil {
					log.Error(err, "failed committing synctree to running")
				}
			}()
		case <-tickerChan:
			if syncRunning.Load() {
				continue
			}
			log.Info("SyncRunning due to ticker")
			treeToCommit := syncTree
			syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
			if err != nil {
				log.Error(err, "failure creating new sync tree after ticker")
				return
			}
			syncRunning.Store(true)
			go func() {
				defer syncRunning.Store(false)
				if err := s.syncToRunning(treeToCommit, true); err != nil {
					log.Error(err, "failed committing synctree to running")
				}
			}()
		}
	}
}

func (s *StreamSync) gnmiSubscribe(subReq *gnmi.SubscribeRequest, updChan chan<- *NotificationData, syncResponse chan<- struct{}) {
	var err error
	log := logger.FromContext(s.ctx)
	log.V(logger.VTrace).Info("starting gnmi subscription", "subscripton", subReq)

	respChan, errChan := s.target.Subscribe(s.ctx, subReq, s.config.Name)

	taskPool := s.vpoolFactory.NewVirtualPool(pool.VirtualTolerant)
	defer func() { taskPool.CloseForSubmit() }()
	taskParams := NewNotificationProcessorTaskParameters(updChan, s.schemaClient, s.notifSendTimeout)

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
				select {
				case syncResponse <- struct{}{}:
				default:
					log.V(logger.VDebug).Info("sync signal already pending, skipping duplicate")
				}

			case *gnmi.SubscribeResponse_Error:
				log.Error(nil, "gnmi subscription error", "error", r.Error.Message)
			}
		}
	}
}

// syncToRunning exports syncTree and applies it to Running. It is called from
// background goroutines spawned by buildTreeSyncWithDatastore; the caller owns
// syncTree exclusively and no mutex is needed.
func (s *StreamSync) syncToRunning(syncTree *tree.RootEntry, logCount bool) error {
	log := logger.FromContext(s.ctx)

	startTime := time.Now()
	result, err := ops.TreeExport(syncTree.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
	log.V(logger.VTrace).Info("exported tree", "tree", result.String())
	if err != nil {
		if errors.Is(err, ops.ErrorIntentNotPresent) {
			log.Info("sync no config changes")
			return nil
		}
		log.Error(err, "sync tree export error")
		return err
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
		return err
	}
	log.V(logger.VTrace).Info("import to running tree done", "duration", time.Since(startTime).String())
	return nil
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
	sendTimeout        time.Duration
}

func NewNotificationProcessorTaskParameters(notificationResult chan<- *NotificationData, scb dsutils.SchemaClientBound, sendTimeout time.Duration) *NotificationProcessorTaskParameters {
	return &NotificationProcessorTaskParameters{
		notificationResult: notificationResult,
		schemaClientBound:  scb,
		sendTimeout:        sendTimeout,
	}
}

func newNotificationProcessorTask(item *gnmi.Notification, params *NotificationProcessorTaskParameters) *notificationProcessorTask {
	return &notificationProcessorTask{
		item:   item,
		params: params,
	}
}

func (t *notificationProcessorTask) sendNotificationData(ctx context.Context, data *NotificationData) error {
	log := logger.FromContext(ctx)
	start := time.Now()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case t.params.notificationResult <- data:
		duration := time.Since(start)
		if duration > notificationSlowSendWarnTime {
			log.Info("slow notification delivery to sync loop", "duration", duration.String())
		}
		return nil
	case <-time.After(t.params.sendTimeout):
		log.Error(context.DeadlineExceeded, "notification delivery timeout, dropping update", "timeout", t.params.sendTimeout.String())
		return nil
	}
}

func (t *notificationProcessorTask) Run(ctx context.Context, _ func(pool.Task) error) error {
	log := logger.FromContext(ctx)
	sn := dsutils.ToSchemaNotification(ctx, t.item)
	// updates
	upds, err := treetypes.ExpandAndConvertIntent(ctx, t.params.schemaClientBound, consts.RunningIntentName, consts.RunningValuesPrio, sn.GetUpdate(), t.item.GetTimestamp())
	if err != nil {
		log.Error(err, "expansion and conversion failed")
	}

	deletes := sdcpb.NewPathSet()
	if len(t.item.GetDelete()) > 0 {
		for _, del := range t.item.GetDelete() {
			deletes.AddPath(dsutils.FromGNMIPath(t.item.GetPrefix(), del).StripPathElemPrefixPath())
		}
	}

	return t.sendNotificationData(ctx, &NotificationData{
		updates: upds,
		deletes: deletes,
	})
}
