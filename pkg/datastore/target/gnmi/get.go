package gnmi

import (
	"context"
	"sync"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	dsutils "github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type GetSync struct {
	config        *config.SyncProtocol
	target        GetTarget
	cancel        context.CancelFunc
	runningStore  types.RunningStore
	ctx           context.Context
	schemaClient  dsutils.SchemaClientBound
	paths         []*sdcpb.Path
	syncTree      *tree.RootEntry
	syncTreeMutex *sync.Mutex
}

func NewGetSync(ctx context.Context, target GetTarget, c *config.SyncProtocol, runningStore types.RunningStore, schemaClient dsutils.SchemaClientBound) (*GetSync, error) {
	ctx, cancel := context.WithCancel(ctx)

	paths := make([]*sdcpb.Path, 0, len(c.Paths))
	// iterate referenced paths
	for _, p := range c.Paths {
		path, err := sdcpb.ParsePath(p)
		if err != nil {
			cancel()
			return nil, err
		}
		// add the parsed path
		paths = append(paths, path)
	}

	return &GetSync{
		config:        c,
		target:        target,
		cancel:        cancel,
		runningStore:  runningStore,
		ctx:           ctx,
		schemaClient:  schemaClient,
		paths:         paths,
		syncTreeMutex: &sync.Mutex{},
	}, nil
}

func (s *GetSync) syncConfig() (*sdcpb.GetDataRequest, error) {
	// iterate syncConfig

	req := &sdcpb.GetDataRequest{
		Name:     s.config.Name,
		Path:     s.paths,
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
	s.syncTreeMutex.Lock()
	defer s.syncTreeMutex.Unlock()

	// execute gnmi get
	resp, err := s.target.Get(s.ctx, req)
	if err != nil {
		log.Errorf("sync error: %v", err)
		return
	}

	s.syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Errorf("sync newemptytree error: %v", err)
		return
	}

	// process Noifications
	err = s.processNotifications(resp.GetNotification())
	if err != nil {
		log.Errorf("sync process notifications error: %v", err)
		return
	}

	result, err := s.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
	if err != nil {
		log.Errorf("sync tree export error: %v", err)
		return
	}

	err = s.runningStore.ApplyToRunning(s.ctx, s.paths, proto.NewProtoTreeImporter(result))
	if err != nil {
		log.Errorf("sync import to running error: %v", err)
		return
	}
}

type GetTarget interface {
	Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error)
}

func (s *GetSync) processNotifications(n []*sdcpb.Notification) error {
	ts := time.Now().Unix()
	uif := treetypes.NewUpdateInsertFlags()

	for _, noti := range n {
		// updates
		upds, err := treetypes.ExpandAndConvertIntent(s.ctx, s.schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, noti.Update, ts)
		if err != nil {
			log.Errorf("sync expanding error: %v", err)
			continue
		}

		for idx2, upd := range upds {
			_ = idx2
			_, err = s.syncTree.AddUpdateRecursive(s.ctx, upd.Path(), upd, uif)
			if err != nil {
				log.Errorf("sync process notifications error: %v, continuing", err)
			}
		}
	}

	return nil
}
