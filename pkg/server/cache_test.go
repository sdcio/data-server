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
	"github.com/sdcio/sdc-protos/tree_persist"
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

	var _ cache.Client = s.cacheClient //nolint:staticcheck // explicit interface assertion is the point of this test
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
				Type:      "config-server",
				Address:   "localhost:50051",
			},
		},
	}

	if err := s.createConfigServerCacheClient(context.Background()); err != nil {
		t.Fatalf("createConfigServerCacheClient() error = %v", err)
	}

	ctx := context.Background()
	if err := s.cacheClient.InstanceIntentModify(ctx, "any-cache", &tree_persist.Intent{IntentName: "any-intent"}); err != nil {
		t.Errorf("InstanceIntentModify() error = %v, want nil", err)
	}
	if err := s.cacheClient.InstanceIntentDelete(ctx, "any-cache", "any-intent", false); err != nil {
		t.Errorf("InstanceIntentDelete() error = %v, want nil", err)
	}
}
