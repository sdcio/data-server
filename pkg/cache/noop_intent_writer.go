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

package cache

import (
	"context"

	"github.com/sdcio/sdc-protos/tree_persist"
)

// noopIntentWriter is a generic IntentWriter for backends that don't own
// real-Intent writes (e.g. config-server, where kube-api is the sole
// writer). It is composed into a full Client at the one legitimate
// type-switch point, Server.createCacheClient, rather than living as
// no-op-shaped methods inside the read-only backend's own type — that keeps
// the backend's type signature honest about what it can actually do, and
// gives any future read-only backend a name to compose instead of writing
// its own no-op boilerplate.
type noopIntentWriter struct{}

func (noopIntentWriter) InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	return nil
}

func (noopIntentWriter) InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error {
	return nil
}
