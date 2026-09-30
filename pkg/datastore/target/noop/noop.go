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

package noop

import (
	"context"
	"time"

	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
)

type noopTarget struct {
	name         string
	runningStore types.RunningStore
}

func NewNoopTarget(_ context.Context, name string, runningStore types.RunningStore) (*noopTarget, error) {
	nt := &noopTarget{
		name:         name,
		runningStore: runningStore,
	}
	return nt, nil
}

// AddSyncs discards every configured sync entry: the noop target never talks
// to a device, so there is nothing to actually sync. Each discarded entry
// immediately satisfies that entry's Synced gate on the RunningStore, since a
// noop-backed datastore has no device sync to wait for.
func (t *noopTarget) AddSyncs(ctx context.Context, sps ...*config.SyncProtocol) error {
	logf.FromContext(ctx).V(1).Info("AddSyncs: discarding sync config (noop target)", "count", len(sps))
	for _, sp := range sps {
		t.runningStore.MarkSynced(sp.Name)
	}
	return nil
}

func (t *noopTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	result := &sdcpb.GetDataResponse{
		Notification: make([]*sdcpb.Notification, 0, len(req.GetPath())),
	}
	for _, p := range req.GetPath() {
		result.Notification = append(result.Notification, &sdcpb.Notification{
			Timestamp: time.Now().UnixNano(),
			Update: []*sdcpb.Update{
				{
					Path: p,
				},
			},
		})
	}

	return result, nil
}

func (t *noopTarget) Set(ctx context.Context, source types.TargetSource) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx).WithName("Set")
	ctx = logf.IntoContext(ctx, log)

	upds, err := source.ToProtoUpdates(ctx, true)
	if err != nil {
		return nil, err
	}

	deletes, err := source.ToProtoDeletes(ctx)
	if err != nil {
		return nil, err
	}

	result := &sdcpb.SetDataResponse{
		Response: make([]*sdcpb.UpdateResult, 0,
			len(upds)+len(deletes)),
		Timestamp: time.Now().UnixNano(),
	}

	for _, upd := range upds {
		result.Response = append(result.Response, &sdcpb.UpdateResult{
			Path: upd.GetPath(),
			Op:   sdcpb.UpdateResult_UPDATE,
		})
	}
	for _, p := range deletes {
		result.Response = append(result.Response, &sdcpb.UpdateResult{
			Path: p,
			Op:   sdcpb.UpdateResult_DELETE,
		})
	}
	return result, nil
}

func (t *noopTarget) Status() *types.TargetStatus {
	return &types.TargetStatus{
		Status: sdcpb.TargetStatus_CONNECTED,
	}
}

func (t *noopTarget) Close(ctx context.Context) error { return nil }
