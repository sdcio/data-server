package netconf

import (
	"context"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type NetconfSync interface {
	Start() error
	Stop() error
}

type NetconfSyncImpl struct {
	config       *config.SyncProtocol
	cancel       context.CancelFunc
	ctx          context.Context
	target       GetXMLImporter
	paths        []*sdcpb.Path
	targetName   string
	runningStore types.RunningStore
}

func NewNetconfSyncImpl(ctx context.Context, targetName string, target GetXMLImporter, c *config.SyncProtocol, runningStore types.RunningStore) (*NetconfSyncImpl, error) {
	ctx, cancel := context.WithCancel(ctx)

	// add the sync name to the logger values
	log := logger.FromContext(ctx).WithValues("sync", c.Name)
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

	return &NetconfSyncImpl{
		config:       c,
		cancel:       cancel,
		ctx:          ctx,
		targetName:   targetName,
		runningStore: runningStore,
		target:       target,
		paths:        paths,
	}, nil
}

func (s *NetconfSyncImpl) Start() error {
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

	go s.internalSync(req)

	go func() {
		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				err := s.internalSync(req)
				if err != nil {
					log.Error(err, "failed syncing")
				}
			}
		}
	}()

	return nil

}

func (s *NetconfSyncImpl) syncConfig() (*sdcpb.GetDataRequest, error) {
	// init a DataRequest
	req := &sdcpb.GetDataRequest{
		Name:     s.targetName,
		Path:     s.paths,
		DataType: sdcpb.DataType_CONFIG,
	}
	return req, nil
}

func (s *NetconfSyncImpl) internalSync(req *sdcpb.GetDataRequest) error {
	// execute netconf get
	importer, err := s.target.GetImportAdapter(s.ctx, req)
	if err != nil {
		return err
	}

	return s.runningStore.ApplyToRunning(s.ctx, s.paths, importer)
}

func (s *NetconfSyncImpl) Stop() error {
	log := logger.FromContext(s.ctx)
	log.Info("Stopping Sync")
	s.cancel()
	return nil
}

type GetXMLImporter interface {
	GetImportAdapter(ctx context.Context, req *sdcpb.GetDataRequest) (importer.ImportConfigAdapter, error)
}
