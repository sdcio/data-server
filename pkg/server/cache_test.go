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
	"errors"
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

// TestCreateConfigServerCacheClient_WritesReachTheClientSeam verifies the
// config-server case composes a Client whose IntentModify/IntentDelete are
// real writes reaching the GRPCConfigClient seam (see
// pkg/cache/docs/adr/0003-config-server-write-path-real-last-applied-writes.md)
// rather than the generic no-op writer other backends fall back to. It only
// exercises the malformed-name fast path — asserting ErrMalformedDatastoreName
// rather than the generic noop nil — since anything past that would need a
// live config-server connection to answer.
func TestCreateConfigServerCacheClient_WritesReachTheClientSeam(t *testing.T) {
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
	const malformed = "no-dot-here"
	if err := s.cacheClient.InstanceIntentModify(ctx, malformed, &tree_persist.Intent{IntentName: "any-intent"}); !errors.Is(err, cache.ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentModify() error = %v, want ErrMalformedDatastoreName", err)
	}
	if err := s.cacheClient.InstanceIntentDelete(ctx, malformed, "any-intent", false); !errors.Is(err, cache.ErrMalformedDatastoreName) {
		t.Errorf("InstanceIntentDelete() error = %v, want ErrMalformedDatastoreName", err)
	}
}
