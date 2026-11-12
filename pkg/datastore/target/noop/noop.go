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

	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
)

type noopTarget struct {
	name string
}

func NewNoopTarget(_ context.Context, name string) (*noopTarget, error) {
	nt := &noopTarget{
		name: name,
	}
	return nt, nil
}

func (t *noopTarget) AddSyncs(ctx context.Context, sps ...*config.SyncProtocol) error {
	logf.FromContext(ctx).V(1).Info("AddSyncs: discarding sync config (noop target)", "count", len(sps))
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

func (t *noopTarget) Set(ctx context.Context, source types.TargetSource) (*types.SetResult, error) {
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

	log.V(logf.VDebug).Info("noop set", "updates", len(upds), "deletes", len(deletes))
	return &types.SetResult{}, nil
}

func (t *noopTarget) Status() *types.TargetStatus {
	return &types.TargetStatus{
		Status: api.ConnectionConnected,
	}
}

func (t *noopTarget) Close(ctx context.Context) error { return nil }
