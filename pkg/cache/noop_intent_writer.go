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

// noopIntentWriter is an IntentWriter whose modify and delete always succeed.
// Read-only backends compose it when another system owns real-Intent writes.
type noopIntentWriter struct{}

func (noopIntentWriter) InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error {
	return nil
}

func (noopIntentWriter) InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error {
	return nil
}
