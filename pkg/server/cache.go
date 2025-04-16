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
	log "github.com/sirupsen/logrus"

	"github.com/sdcio/data-server/pkg/cache"
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
	}
}

func (s *Server) createLocalCacheClient(_ context.Context) error {
	var err error
	log.Infof("initializing local cache client")
	s.cacheClient, err = cache.NewLocalCache(&cconfig.CacheConfig{
		StoreType: s.config.Cache.StoreType,
		Dir:       s.config.Cache.Dir,
	})
	return err
}
