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
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
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

	// add the sync name to the logger values
	log := logger.FromContext(ctx).WithValues("sync", c.Name).WithValues("type", "GET")
	ctx = logger.IntoContext(ctx, log)

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
	log := logger.FromContext(s.ctx)
	log.Info("Stopping Sync", "sync", s.config.Name)
	s.cancel()
	return nil
}

func (s *GetSync) Name() string {
	return s.config.Name
}

func (s *GetSync) Start() error {
	log := logger.FromContext(s.ctx)
	log.Info("Starting Sync")

	req, err := s.syncConfig()
	if err != nil {
		return err
	}

	if s.ctx.Err() != nil {
		// stopping sync due to context canceled
		return nil
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
	log := logger.FromContext(s.ctx)
	log.V(logger.VDebug).Info("syncing")

	s.syncTreeMutex.Lock()
	defer s.syncTreeMutex.Unlock()

	// execute gnmi get
	resp, err := s.target.Get(s.ctx, req)
	if err != nil {
		log.Error(err, "error performing gnmi get from target")
		return
	}

	s.syncTree, err = s.runningStore.NewEmptyTree(s.ctx)
	if err != nil {
		log.Error(err, "failure creating new synctree")
		return
	}

	// process Noifications
	err = s.processNotifications(resp.GetNotification())
	if err != nil {
		log.Error(err, "failed processing notifications")
		return
	}

	result, err := s.syncTree.TreeExport(tree.RunningIntentName, tree.RunningValuesPrio, false)
	if err != nil {
		log.Error(err, "failure exporting synctree")
		return
	}

	if log := log.V(logger.VTrace); log.Enabled() {
		data, _ := protojson.Marshal(result)
		log.Info("sync content", "data", string(data))
	}

	err = s.runningStore.ApplyToRunning(s.ctx, s.paths, proto.NewProtoTreeImporter(result))
	if err != nil {
		log.Error(err, "failure importing synctree export into running")
		return
	}
	log.V(logger.VDebug).Info("syncing done")
}

type GetTarget interface {
	Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error)
}

func (s *GetSync) processNotifications(n []*sdcpb.Notification) error {
	log := logger.FromContext(s.ctx)
	uif := treetypes.NewUpdateInsertFlags()

	for _, noti := range n {
		// updates
		upds, err := treetypes.ExpandAndConvertIntent(s.ctx, s.schemaClient, tree.RunningIntentName, tree.RunningValuesPrio, noti.Update, noti.GetTimestamp())
		if err != nil {
			log.Error(err, "failure expanding and converting notification")
			continue
		}

		for idx2, upd := range upds {
			_ = idx2
			_, err = s.syncTree.AddUpdateRecursive(s.ctx, upd.GetPath(), upd.GetUpdate(), uif)
			if err != nil {
				log.Error(err, "failure adding update to synctree")
			}
		}
	}

	return nil
}
