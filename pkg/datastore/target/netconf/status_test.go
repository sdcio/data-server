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
	"errors"
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/datastore/target/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// Get and Set reject requests via Status().Err() before touching any other
// field, so both must survive a nil receiver rather than panicking.
func Test_ncTarget_Status_NilReceiver(t *testing.T) {
	var target *ncTarget

	st := target.Status()
	if st.Status != sdcpb.TargetStatus_NOT_CONNECTED {
		t.Errorf("expected NOT_CONNECTED, got %v", st.Status)
	}
	if st.IsConnected() {
		t.Error("expected a nil target to report not connected")
	}
	if err := st.Err(); !errors.Is(err, types.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func Test_ncTarget_Status_UninitializedDriver(t *testing.T) {
	target := &ncTarget{name: "dev1"}

	err := target.Status().Err()
	if !errors.Is(err, types.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
	if !strings.Contains(err.Error(), "connection not initialized") {
		t.Errorf("expected the status details to reach the error, got %q", err.Error())
	}
}
