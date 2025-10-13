package gnmi

import (
	"context"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type GetSync struct {
	config       *config.SyncProtocol
	target       GetTarget
	cancel       context.CancelFunc
	runningStore types.RunningStore
	ctx          context.Context
	schemaClient dsutils.SchemaClientBound
}

func NewGetSync(ctx context.Context, target GetTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound) *GetSync {
	ctx, cancel := context.WithCancel(ctx)

	return &GetSync{
		config:       c,
		target:       target,
		cancel:       cancel,
		runningStore: runningStore,
		ctx:          ctx,
		schemaClient: schemaClient,
	}
}

func (s *GetSync) syncConfig() (*sdcpb.GetDataRequest, error) {
	// iterate syncConfig
	paths := make([]*sdcpb.Path, 0, len(s.config.Paths))
	// iterate referenced paths
	for _, p := range s.config.Paths {
		path, err := sdcpb.ParsePath(p)
		if err != nil {
			return nil, err
		}
		// add the parsed path
		paths = append(paths, path)
	}

	req := &sdcpb.GetDataRequest{
		Name:     s.config.Name,
		Path:     paths,
		DataType: sdcpb.DataType_CONFIG,
		Encoding: sdcpb.Encoding(utils.ParseSdcpbEncoding(s.config.Encoding)),
	}
	return req, nil
}

func (s *GetSync) Stop() error {
	// TODO
	s.cancel()
	return nil
}

func (s *GetSync) Start() error {
	req, err := s.syncConfig()
	if err != nil {
		return err
	}

	go s.internalGetSync(req)

	go func() {
		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.internalGetSync(req)
			}
		}
	}()

	return nil
}

func (s *GetSync) internalGetSync(req *sdcpb.GetDataRequest) {
	// execute gnmi get
	resp, err := s.target.Get(s.ctx, req)
	if err != nil {
		log.Errorf("sync error: %v", err)
		return
	}
	syncTree, err := s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Errorf("sync newemptytree error: %v", err)
		return
	}

	// process Noifications
	deletes, err := processNotifications(s.ctx, resp.GetNotification(), s.schemaClient, syncTree)
	if err != nil {
		log.Errorf("sync process notifications error: %v", err)
		return
	}

	result, err := syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
	if err != nil {
		log.Errorf("sync tree export error: %v", err)
		return
	}
	// add also the deletes to the export
	result.ExplicitDeletes = deletes

	err = s.runningStore.ApplyToRunning(s.ctx, result)
	if err != nil {
		log.Errorf("sync import to running error: %v", err)
		return
	}
}

type GetTarget interface {
	Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error)
}

func processNotifications(ctx context.Context, n []*sdcpb.Notification, schemaClient dsutils.SchemaClientBound, syncTree *tree.RootEntry) ([]*sdcpb.Path, error) {

	ts := time.Now().Unix()
	uif := treetypes.NewUpdateInsertFlags()

	deletes := []*sdcpb.Path{}

	for _, noti := range n {
		// updates
		upds, err := treetypes.ExpandAndConvertIntent(ctx, schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, noti.Update, ts)
		if err != nil {
			log.Errorf("sync expanding error: %v", err)
			return nil, err
		}

		for idx2, upd := range upds {
			_ = idx2
			_, err = syncTree.AddUpdateRecursive(ctx, upd.Path(), upd, uif)
			if err != nil {
				log.Errorf("sync process notifications error: %v", err)
				return nil, err
			}

		}
		// deletes
		deletes = append(deletes, noti.GetDelete()...)
	}
	return deletes, nil
}
