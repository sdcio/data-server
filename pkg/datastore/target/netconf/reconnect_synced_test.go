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

package netconf

import (
	"context"
	"sync"
	"testing"

	"github.com/sdcio/data-server/mocks/mocknetconf"
	"github.com/sdcio/data-server/pkg/config"
	"go.uber.org/mock/gomock"
)

// TestNcTarget_Reconnect_DoesNotRecreateSyncs guards the invariant the ticket
// relies on for the Synced latch surviving a target reconnect: AddSyncs (and
// therefore sync object construction / re-registration) is only ever called
// once, at target construction. reconnect() only replaces the underlying
// NETCONF driver; it must never touch t.syncs or call AddSyncs again.
//
// A live driver (IsAlive() == true) makes reconnect() a no-op via the early
// "already connected" return, which is exactly the branch that must never
// re-run sync setup.
func TestNcTarget_Reconnect_DoesNotRecreateSyncs(t *testing.T) {
	ctrl := gomock.NewController(t)
	driver := mocknetconf.NewMockDriver(ctrl)
	driver.EXPECT().IsAlive().Return(true).AnyTimes()

	target := &ncTarget{
		name:      "dev1",
		driver:    driver,
		m:         new(sync.Mutex),
		syncs:     map[string]NetconfSync{"s1": nil},
		sbiConfig: &config.SBI{},
	}
	syncsBefore := target.syncs

	target.reconnect(context.Background())

	if len(target.syncs) != len(syncsBefore) {
		t.Errorf("reconnect() changed the syncs set: before=%v after=%v", syncsBefore, target.syncs)
	}
	for name := range syncsBefore {
		if _, ok := target.syncs[name]; !ok {
			t.Errorf("reconnect() dropped sync %q", name)
		}
	}
}
