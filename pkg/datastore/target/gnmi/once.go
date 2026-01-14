package gnmi

import (
	"context"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/logger"
)

type OnceSync struct {
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	ctx          context.Context
	vpoolFactory pool.VirtualPoolFactory
}

func NewOnceSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore, vpoolFactory pool.VirtualPoolFactory) *OnceSync {
	ctx, cancel := context.WithCancel(ctx)
	// add the sync name to the logger values
	log := logger.FromContext(ctx).WithValues("sync", c.Name)
	ctx = logger.IntoContext(ctx, log)

	return &OnceSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
		vpoolFactory: vpoolFactory,
		ctx:          ctx,
	}
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

	// initial subscribe ONCE
	go s.target.Subscribe(s.ctx, subReq, s.config.Name)
	// periodic subscribe ONCE
	go func(interval time.Duration, name string) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.target.Subscribe(s.ctx, subReq, name)
			}
		}
	}(s.config.Interval, s.config.Name)
	return nil
}

func (s *OnceSync) Stop() error {
	log := logger.FromContext(s.ctx)
	log.Info("Stopping Sync", "sync", s.config.Name)

	s.cancel()
	return nil
}
