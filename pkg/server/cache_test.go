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
	"testing"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
)

func TestCreateConfigServerCacheClient(t *testing.T) {
	s := &Server{
		config: &config.Config{
			Cache: &config.CacheConfig{
				Type:      "config-server",
				Address:   "localhost:50051",
				Namespace: "sdcio",
			},
		},
	}

	err := s.createConfigServerCacheClient(context.Background())
	if err != nil {
		t.Fatalf("createConfigServerCacheClient() error = %v", err)
	}

	if _, ok := s.cacheClient.(*cache.ConfigServerCache); !ok {
		t.Fatalf("cacheClient = %T, want *cache.ConfigServerCache", s.cacheClient)
	}
}
