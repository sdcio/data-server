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

package gnmi_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// capturingGetTarget implements gnmi.GetTarget.
// The first Get call is captured to the channel, then an error is returned so
// that internalGetSync exits early without touching RunningStore.
type capturingGetTarget struct {
	captured chan *sdcpb.GetDataRequest
}

func (c *capturingGetTarget) Get(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	select {
	case c.captured <- req:
	default:
	}
	return nil, fmt.Errorf("capture-and-stop")
}

// TestGetSync_syncConfig_UsesDataTypeConfig asserts that GetSync requests
// DataType_CONFIG (not DataType_ALL) when it issues the periodic Get to the device.
func TestGetSync_syncConfig_UsesDataTypeConfig(t *testing.T) {
	captured := make(chan *sdcpb.GetDataRequest, 1)
	target := &capturingGetTarget{captured: captured}

	syncConf := &config.SyncProtocol{
		Name:     "test-get-sync",
		Interval: time.Hour, // long enough that only the initial sync fires
		Encoding: "JSON_IETF",
	}

	// RunningStore is never accessed because Get() returns an error, causing
	// internalGetSync to return early. Passing nil satisfies the interface.
	var rs targettypes.RunningStore

	gs, err := gnmi.NewGetSync(context.Background(), target, syncConf, rs, nil)
	if err != nil {
		t.Fatalf("NewGetSync: %v", err)
	}

	if err := gs.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer gs.Stop()

	select {
	case req := <-captured:
		if req.DataType != sdcpb.DataType_CONFIG {
			t.Errorf("syncConfig: want DataType_CONFIG (%v), got %v",
				sdcpb.DataType_CONFIG, req.DataType)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for GetSync to issue its first Get request")
	}
}
