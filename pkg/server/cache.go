// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"fmt"
	"os"
	"time"

	cconfig "github.com/sdcio/cache/pkg/config"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/cache/configserver"
	logf "github.com/sdcio/logger"
)

func (s *Server) createCacheClient(ctx context.Context) {
	log := logf.FromContext(ctx)
	log = log.WithValues("cache-type", s.config.Cache.Type)
	ctx = logf.IntoContext(ctx, log)
START:
	var err error
	switch s.config.Cache.Type {
	default:
		log.Error(nil, "invalid cache type")
		fmt.Fprintf(os.Stderr, "unknown cache type: %s", s.config.Cache.Type)
		os.Exit(1)
	case "local":
		err = s.createLocalCacheClient(ctx)
		if err != nil {
			log.Error(err, "failed to initialize a local cache client")
			time.Sleep(time.Second)
			goto START
		}
	case "config-server":
		err = s.createConfigServerCacheClient(ctx)
		if err != nil {
			log.Error(err, "failed to initialize a config-server cache client")
			time.Sleep(time.Second)
			goto START
		}
	}
}

func (s *Server) createLocalCacheClient(ctx context.Context) error {
	log := logf.FromContext(ctx)
	var err error
	log.Info("initializing local cache client", "store-type", s.config.Cache.StoreType, "directory", s.config.Cache.Dir)
	s.cacheClient, err = cache.NewLocalCache(&cconfig.CacheConfig{
		StoreType: s.config.Cache.StoreType,
		Dir:       s.config.Cache.Dir,
	})
	log.Info("local cache created")
	return err
}

// createConfigServerCacheClient wires the colocated config-server
// controller's local ConfigReadService (ticket 07) in as the real-intent
// backend, per the ADR's "New cache.Client implementation" section:
// real Intents are read through configserver.GRPCConfigReader, "running"
// stays independent in-memory state owned by cache.ConfigServerCache itself.
func (s *Server) createConfigServerCacheClient(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("initializing config-server cache client", "address", s.config.Cache.Address, "namespace", s.config.Cache.Namespace)
	cc, err := configserver.Dial(s.config.Cache.Address)
	if err != nil {
		return err
	}
	reader := configserver.NewGRPCConfigReader(cc)
	s.cacheClient = cache.NewConfigServerClient(reader)
	log.Info("config-server cache client created")
	return nil
}
