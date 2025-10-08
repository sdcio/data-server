package gnmi

import (
	"context"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
)

type OnceSync struct {
	config       *config.SyncProtocol
	target       SyncTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	ctx          context.Context
}

func NewOnceSync(ctx context.Context, target SyncTarget, c *config.SyncProtocol, runningStore types.RunningStore) *OnceSync {
	ctx, cancel := context.WithCancel(ctx)
	return &OnceSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
	}
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
		gapi.DataTypeCONFIG(),
	)
	subReq, err := gapi.NewSubscribeRequest(opts...)
	if err != nil {
		return nil, err
	}
	return subReq, nil
}

func (s *OnceSync) Start() error {

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
	// TODO
	s.cancel()
	return nil
}
