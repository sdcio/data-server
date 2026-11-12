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

	// paths are the configured sync paths; the initial sync after a
	// resubscribe replaces exactly these in the running tree.
	paths []*sdcpb.Path

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

	subReq, err := s.syncConfig()
	if err != nil {
		return err
	}

	s.paths = make([]*sdcpb.Path, 0, len(s.config.Paths))
	for _, p := range s.config.Paths {
		sp, err := sdcpb.ParsePath(p)
		if err != nil {
			// the path is still subscribed, but is not replaced on resubscribe
			log.Error(err, "sync path not usable for resync", "path", p)
			continue
		}
		s.paths = append(s.paths, sp)
	}

	// start the gnmi subscribe request, that also used the pool for
	go s.gnmiSubscribe(subReq, updChan)
	//
	go s.buildTreeSyncWithDatastore(updChan)

	return nil
}

// buildTreeSyncWithDatastore accumulates incoming notifications into a local
// syncTree and commits it to Running on SyncResponse and every ticker tick.
//
// The first SyncResponse after a resubscribe marker is a full snapshot of the
// device: it is committed replacing the configured sync paths, and ticker
// commits are held back until it arrives so a partial snapshot never replaces
// running.
//
// The commit is handed off to a background goroutine so the main select loop
// never blocks on ApplyToRunning. This prevents notifications from timing out
// and being silently dropped while a slow performRevert holds the goroutine
// (see GitHub issue #440). Commits still apply to Running in the order they
// were started.
func (s *StreamSync) buildTreeSyncWithDatastore(cUS <-chan *NotificationData) {
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

	// inflight counts running syncToRunning goroutines. The ticker case skips
	// while any is running; the SyncResponse case always proceeds.
	var inflight atomic.Int32

	// prevDone is closed when the most recently started commit has finished;
	// the next commit waits on it so commits reach Running in start order.
	prevDone := make(chan struct{})
	close(prevDone)

	// awaitingFullSync is true from a resubscribe marker until the following
	// SyncResponse.
	awaitingFullSync := false

	// commit hands the accumulated syncTree to a background goroutine and
	// continues with a fresh tree. It reports false when no fresh tree could
	// be created.
	commit := func(replacePaths []*sdcpb.Path) bool {
		treeToCommit := syncTree
		next, err := s.runningStore.NewEmptyTree(s.ctx)
		if err != nil {
			log.Error(err, "failure creating new sync tree")
			return false
		}
		syncTree = next

		waitFor := prevDone
		done := make(chan struct{})
		prevDone = done
		inflight.Add(1)
		go func() {
			defer close(done)
			defer inflight.Add(-1)
			<-waitFor
			if err := s.syncToRunning(treeToCommit, replacePaths, true); err != nil {
				log.Error(err, "failed committing synctree to running")
			}
		}()
		return true
	}

	for {
		select {
		case <-s.ctx.Done():
			log.V(logger.VDebug).Info("stopping sync due to context done")
			return
		case noti, ok := <-cUS:
			if !ok {
				return
			}
			switch noti.marker {
			case markerResubscribed:
				// anything accumulated so far is superseded by the snapshot
				// the new subscription delivers
				syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
				if err != nil {
					log.Error(err, "failure creating new sync tree after resubscribe")
					return
				}
				awaitingFullSync = true
				continue
			case markerSyncResponse:
				var replacePaths []*sdcpb.Path
				if awaitingFullSync {
					replacePaths = s.paths
					awaitingFullSync = false
				}
				if !commit(replacePaths) {
					return
				}
				if tickerChan == nil {
					t := time.NewTicker(5 * time.Second)
					defer t.Stop()
					tickerChan = t.C
				}
				continue
			}
			err := syncTree.AddUpdatesRecursive(s.ctx, noti.updates, uif)
			if err != nil {
				log.Error(err, "failed adding update to synctree")
			}
			syncTree.GetTreeContext().ExplicitDeletes().Add(consts.RunningIntentName, consts.RunningValuesPrio, noti.deletes)
		case <-tickerChan:
			if inflight.Load() > 0 || awaitingFullSync {
				continue
			}
			log.Info("SyncRunning due to ticker")
			if !commit(nil) {
				return
			}
		}
	}
}

// gnmiSubscribe forwards the target's subscription to updChan. The target
// resubscribes on its own after a stream failure and reports each failure on
// its error channel, so errors there are logged and the loop keeps running.
// A failure is announced downstream with a resubscribe marker, and the end of
// every initial sync with a sync-response marker. Both are sent only after all
// earlier notifications have been delivered, so they order behind them.
func (s *StreamSync) gnmiSubscribe(subReq *gnmi.SubscribeRequest, updChan chan<- *NotificationData) {
	var err error
	log := logger.FromContext(s.ctx)
	log.V(logger.VTrace).Info("starting gnmi subscription", "subscripton", subReq)

	respChan, errChan := s.target.Subscribe(s.ctx, subReq, s.config.Name)

	taskPool := s.vpoolFactory.NewVirtualPool(pool.VirtualTolerant)
	defer func() { taskPool.CloseForSubmit() }()
	taskParams := NewNotificationProcessorTaskParameters(updChan, s.schemaClient, s.notifSendTimeout)

	// sendMarker waits for all submitted notification tasks to finish, then
	// sends the marker. It reports false when the sync is stopping.
	sendMarker := func(m notificationMarker) bool {
		taskPool.CloseAndWait()
		taskPool = s.vpoolFactory.NewVirtualPool(pool.VirtualTolerant)
		select {
		case updChan <- &NotificationData{marker: m}:
			return true
		case <-s.ctx.Done():
			return false
		}
	}

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
				log.Error(err, "stream sync interrupted, waiting for resubscribe")
				syncStartTime = time.Now()
				if !sendMarker(markerResubscribed) {
					return
				}
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
				if !sendMarker(markerSyncResponse) {
					return
				}

			case *gnmi.SubscribeResponse_Error:
				subscribeErr := r.Error //nolint:staticcheck // gnmi SubscribeResponse_Error deprecated upstream without replacement.
				msg := ""
				if subscribeErr != nil {
					msg = subscribeErr.GetMessage()
				}
				log.Error(nil, "gnmi subscription error", "error", msg)
			}
		}
	}
}

// syncToRunning exports syncTree and applies it to Running. It is called from
// background goroutines spawned by buildTreeSyncWithDatastore; the caller owns
// syncTree exclusively and no mutex is needed.
//
// replacePaths are deleted from Running before the import, so Running under
// those paths ends up equal to syncTree. An empty syncTree still applies them.
func (s *StreamSync) syncToRunning(syncTree *tree.RootEntry, replacePaths []*sdcpb.Path, logCount bool) error {
	log := logger.FromContext(s.ctx)

	startTime := time.Now()
	result, err := ops.TreeExport(syncTree.Entry, consts.RunningIntentName, consts.RunningValuesPrio, false)
	log.V(logger.VTrace).Info("exported tree", "tree", result.String())
	if err != nil {
		if errors.Is(err, ops.ErrorIntentNotPresent) {
			if len(replacePaths) == 0 {
				log.Info("sync no config changes")
				return nil
			}
			log.Info("sync found no config under the synced paths, clearing them in running")
			return s.runningStore.ApplyToRunning(s.ctx, replacePaths, nil)
		}
		log.Error(err, "sync tree export error")
		return err
	}
	// extract the explicit deletes
	deletes := append(append([]*sdcpb.Path{}, replacePaths...), result.ExplicitDeletes...)
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

// notificationMarker tags a NotificationData that carries a stream event
// instead of notification content.
type notificationMarker int

const (
	markerNone notificationMarker = iota
	// markerResubscribed: the subscription failed and restarts; the next
	// initial sync is a full snapshot.
	markerResubscribed
	// markerSyncResponse: the target finished sending the initial state.
	markerSyncResponse
)

type NotificationData struct {
	updates []*treetypes.PathAndUpdate
	deletes *sdcpb.PathSet
	marker  notificationMarker
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
