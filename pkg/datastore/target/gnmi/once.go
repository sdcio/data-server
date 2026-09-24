package gnmi

import (
	"context"
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
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type OnceSync struct {
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	ctx          context.Context
	schemaClient dsutils.SchemaClientBound
	paths        []*sdcpb.Path

	cycleRunning atomic.Bool
}

func NewOnceSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound, _ pool.VirtualPoolFactory) (*OnceSync, error) {
	ctx, cancel := context.WithCancel(ctx)
	log := logger.FromContext(ctx).WithValues("sync", c.Name).WithValues("type", "ONCE")
	ctx = logger.IntoContext(ctx, log)

	paths := make([]*sdcpb.Path, 0, len(c.Paths))
	for _, p := range c.Paths {
		path, err := sdcpb.ParsePath(p)
		if err != nil {
			cancel()
			return nil, err
		}
		paths = append(paths, path)
	}

	return &OnceSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
		ctx:          ctx,
		schemaClient: schemaClient,
		paths:        paths,
	}, nil
}

func (s *OnceSync) Name() string {
	return s.config.Name
}

func (s *OnceSync) syncConfig() (*gnmi.SubscribeRequest, error) {
	opts := make([]gapi.GNMIOption, 0)
	subscriptionOpts := make([]gapi.GNMIOption, 0)
	for _, p := range s.config.Paths {
		subscriptionOpts = append(subscriptionOpts, gapi.Path(p))
	}
	opts = append(opts,
		gapi.EncodingCustom(utils.ParseGnmiEncoding(s.config.Encoding)),
		gapi.SubscriptionListModeONCE(),
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

func (s *OnceSync) Start() error {
	log := logger.FromContext(s.ctx)
	log.Info("Starting Sync")

	subReq, err := s.syncConfig()
	if err != nil {
		return err
	}

	if s.ctx.Err() != nil {
		return nil
	}

	go s.internalOnceCycle(subReq)

	go func() {
		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.internalOnceCycle(subReq)
			}
		}
	}()

	return nil
}

func (s *OnceSync) Stop() error {
	log := logger.FromContext(s.ctx)
	log.Info("Stopping Sync", "sync", s.config.Name)

	s.cancel()
	return nil
}

func (s *OnceSync) internalOnceCycle(subReq *gnmi.SubscribeRequest) {
	if !s.cycleRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.cycleRunning.Store(false)

	log := logger.FromContext(s.ctx)
	log.V(logger.VDebug).Info("syncing")

	syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Error(err, "failure creating new synctree")
		return
	}

	respChan, errChan := s.target.Subscribe(s.ctx, subReq, s.config.Name)
	gotSyncResponse := false

	for {
		select {
		case <-s.ctx.Done():
			return
		case err, ok := <-errChan:
			if !ok {
				return
			}
			if err != nil {
				log.Error(err, "error performing gnmi subscribe once from target")
				return
			}
		case resp, ok := <-respChan:
			if !ok {
				if !gotSyncResponse {
					log.V(logger.VDebug).Info("subscribe stream closed without SyncResponse")
				}
				return
			}
			switch r := resp.GetResponse().(type) {
			case *gnmi.SubscribeResponse_Update:
				s.applyNotificationUpdates(syncTree, r.Update)
			case *gnmi.SubscribeResponse_SyncResponse:
				gotSyncResponse = true
				if err := applyScopedRefreshFromCycleTree(s.ctx, s.runningStore, syncTree, s.paths); err != nil {
					log.Error(err, "failure applying sync cycle to running")
					return
				}
				s.runningStore.MarkSynced(s.config.Name)
				log.V(logger.VDebug).Info("syncing done")
				return
			case *gnmi.SubscribeResponse_Error:
				subscribeErr := r.Error //nolint:staticcheck // gnmi SubscribeResponse_Error deprecated upstream without replacement.
				msg := ""
				if subscribeErr != nil {
					msg = subscribeErr.GetMessage()
				}
				log.Error(nil, "gnmi subscription error", "error", msg)
				return
			}
		}
	}
}

func (s *OnceSync) applyNotificationUpdates(syncTree *tree.RootEntry, notif *gnmi.Notification) {
	if notif == nil {
		return
	}
	log := logger.FromContext(s.ctx)
	sn := dsutils.ToSchemaNotification(s.ctx, notif)
	uif := treetypes.NewUpdateInsertFlags()

	upds, err := treetypes.ExpandAndConvertIntent(s.ctx, s.schemaClient, consts.RunningIntentName, consts.RunningValuesPrio, sn.GetUpdate(), notif.GetTimestamp())
	if err != nil {
		log.Error(err, "failure expanding and converting notification")
		return
	}

	for _, upd := range upds {
		_, err = ops.AddUpdateRecursive(s.ctx, syncTree.Entry, upd.GetPath(), upd.GetUpdate(), uif)
		if err != nil {
			log.Error(err, "failure adding update to synctree")
		}
	}
}
