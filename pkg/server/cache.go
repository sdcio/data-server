package server

import (
	"context"
	"fmt"
	"os"
	"time"

	cconfig "github.com/iptecharch/cache/pkg/config"
	log "github.com/sirupsen/logrus"

	"github.com/iptecharch/data-server/pkg/cache"
)

func (s *Server) createCacheClient(ctx context.Context) {
START:
	var err error
	switch s.config.Cache.Type {
	default:
		fmt.Fprintf(os.Stderr, "unknown cache type: %s", s.config.Cache.Type)
		os.Exit(1)
	case "local":
		err = s.createLocalCacheClient(ctx)
		if err != nil {
			log.Errorf("failed to initialize a local cache client: %v", err)
			time.Sleep(time.Second)
			goto START
		}
		log.Infof("local cache created")
	case "remote":
		err = s.createRemoteCacheClient(ctx)
		if err != nil {
			log.Errorf("failed to initialize a remote cache client: %v", err)
			time.Sleep(time.Second)
			goto START
		}
		log.Infof("connected to remote cache: %s", s.config.Cache.Address)
	}
}

func (s *Server) createLocalCacheClient(ctx context.Context) error {
	var err error
	log.Infof("initializing local cache client")
	s.cacheClient, err = cache.NewLocalCache(&cconfig.CacheConfig{
		MaxCaches: -1,
		StoreType: s.config.Cache.StoreType,
		Dir:       s.config.Cache.Dir,
	})
	return err
}

func (s *Server) createRemoteCacheClient(ctx context.Context) error {
	log.Infof("initializing remote cache client")
	var err error
	s.cacheClient, err = cache.NewRemoteCache(ctx, s.config.Cache.Address)
	return err
}
