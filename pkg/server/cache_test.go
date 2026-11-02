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
			},
		},
	}

	err := s.createConfigServerCacheClient(context.Background())
	if err != nil {
		t.Fatalf("createConfigServerCacheClient() error = %v", err)
	}

	if s.cacheClient == nil {
		t.Fatal("cacheClient = nil, want a config-server-backed cache.Client")
	}
	if _, ok := s.cacheClient.(cache.IntentReader); !ok {
		t.Fatalf("cacheClient = %T, does not implement cache.IntentReader", s.cacheClient)
	}
}

// TestCreateConfigServerCacheClient_WritesAreNoOps verifies the config-server
// case composes a Client whose IntentModify/IntentDelete behavior is the
// generic no-op (noopIntentWriter) — reachable without ever going through
// the read seam (configserver.LocalConfigReader), since a real backend would
// need network/reader wiring to answer at all.
func TestCreateConfigServerCacheClient_WritesAreNoOps(t *testing.T) {
	s := &Server{
		config: &config.Config{
			Cache: &config.CacheConfig{
				Type:    "config-server",
				Address: "localhost:50051",
			},
		},
	}

	if err := s.createConfigServerCacheClient(context.Background()); err != nil {
		t.Fatalf("createConfigServerCacheClient() error = %v", err)
	}

	if err := s.cacheClient.InstanceIntentModify(context.Background(), "ns1.target1", nil); err != nil {
		t.Errorf("InstanceIntentModify() error = %v, want nil", err)
	}
	if err := s.cacheClient.InstanceIntentDelete(context.Background(), "ns1.target1", "intent1", false); err != nil {
		t.Errorf("InstanceIntentDelete() error = %v, want nil", err)
	}
}
