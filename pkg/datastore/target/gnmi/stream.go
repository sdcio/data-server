package gnmi

import (
	"context"

	"github.com/openconfig/gnmi/proto/gnmi"
	gapi "github.com/openconfig/gnmic/pkg/api"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree"
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

	go func() {
		syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
		if err != nil {
			log.Errorf("sync newemptytree error: %v", err)
			return
		}
		for {
			select {
			case <-s.ctx.Done():
				return
			case err = <-errChan:
				if err != nil {
					log.Errorf("Error stream sync: %s", err)
					return
				}
			case resp, ok := <-respChan:
				if !ok {
					return
				}
				_ = resp
				switch r := resp.GetResponse().(type) {
				case *gnmi.SubscribeResponse_Update:
					sn := dsutils.ToSchemaNotification(r.Update)
					processNotifications(s.ctx, []*sdcpb.Notification{sn}, s.schemaClient, syncTree)
				case *gnmi.SubscribeResponse_SyncResponse:
					log.Info("SyncResponse flag received")
					result, err := syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
					if err != nil {
						log.Errorf("sync tree export error: %v", err)
						return
					}

					err = s.runningStore.ApplyToRunning(s.ctx, result)
					if err != nil {
						log.Errorf("sync import to running error: %v", err)
						return
					}
					syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
					if err != nil {
						log.Errorf("sync newemptytree error: %v", err)
						return
					}
				case *gnmi.SubscribeResponse_Error:
					log.Error(r.Error.Message)
				}
			}
		}
	}()

	return nil
}

type SyncTarget interface {
	Subscribe(ctx context.Context, req *gnmi.SubscribeRequest, subscriptionName string) (chan *gnmi.SubscribeResponse, chan error)
}
