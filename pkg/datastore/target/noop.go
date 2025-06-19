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

package target

import (
	"context"
	"time"

	"github.com/sdcio/data-server/pkg/config"
	logf "github.com/sdcio/data-server/pkg/log"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type noopTarget struct {
	name string
}

func newNoopTarget(_ context.Context, name string) (*noopTarget, error) {
	nt := &noopTarget{
		name: name,
	}
	return nt, nil
}

func (t *noopTarget) Get(_ context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
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

func (t *noopTarget) Set(ctx context.Context, source TargetSource) (*sdcpb.SetDataResponse, error) {

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

func (t *noopTarget) Status() *TargetStatus {
	return &TargetStatus{
		Status: TargetStatusConnected,
	}
}

func (t *noopTarget) Sync(ctx context.Context, _ *config.Sync, syncCh chan *SyncUpdate) {
	log := logf.FromContext(ctx)
	log.Info("starting target sync")
}

func (t *noopTarget) Close() error { return nil }
