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
	"testing"

	"github.com/sdcio/sdc-protos/tree_persist"
)

var _ IntentWriter = noopIntentWriter{}

// TestNoopIntentWriter_AlwaysNil verifies InstanceIntentModify/
// InstanceIntentDelete return nil unconditionally, regardless of input —
// there is no backend to fail against, so there is no error path to
// exercise.
func TestNoopIntentWriter_AlwaysNil(t *testing.T) {
	w := noopIntentWriter{}
	ctx := context.Background()

	if err := w.InstanceIntentModify(ctx, "any-cache", &tree_persist.Intent{IntentName: "any-intent"}); err != nil {
		t.Errorf("InstanceIntentModify() error = %v, want nil", err)
	}
	if err := w.InstanceIntentDelete(ctx, "any-cache", "any-intent", false); err != nil {
		t.Errorf("InstanceIntentDelete() error = %v, want nil", err)
	}
}
