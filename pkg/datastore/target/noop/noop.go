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
	"encoding/json"
	"time"

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
	log := logf.FromContext(ctx)
	for _, sp := range sps {
		jConf, err := json.Marshal(sp)
		if err != nil {
			return err
		}
		log.Info("Sync added", "Config", jConf)
	}
	return nil
}

func (t *noopTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	log := logf.FromContext(ctx).WithName("Get")
	ctx = logf.IntoContext(ctx, log)

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

func (t *noopTarget) Set(ctx context.Context, plan types.SouthboundSetPlan) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx).WithName("Set")

	result := &sdcpb.SetDataResponse{
		Timestamp: time.Now().UnixNano(),
	}

	if gp, ok := plan.GnmiPlan(); ok {
		result.Response = make([]*sdcpb.UpdateResult, 0, len(gp.Updates)+len(gp.Deletes))
		for _, upd := range gp.Updates {
			log.V(logf.VDebug).Info("noop set update", "path", upd.GetPath())
			result.Response = append(result.Response, &sdcpb.UpdateResult{
				Path: upd.GetPath(),
				Op:   sdcpb.UpdateResult_UPDATE,
			})
		}
		for _, p := range gp.Deletes {
			log.V(logf.VDebug).Info("noop set delete", "path", p)
			result.Response = append(result.Response, &sdcpb.UpdateResult{
				Path: p,
				Op:   sdcpb.UpdateResult_DELETE,
			})
		}
		return result, nil
	}

	if _, ok := plan.NetconfPlan(); ok {
		log.V(logf.VDebug).Info("noop set netconf plan received")
		result.Response = []*sdcpb.UpdateResult{}
		return result, nil
	}

	log.V(logf.VDebug).Info("noop set empty plan received")
	result.Response = []*sdcpb.UpdateResult{}
	return result, nil
}

func (t *noopTarget) Status() *types.TargetStatus {
	return &types.TargetStatus{
		Status: types.TargetStatusConnected,
	}
}

func (t *noopTarget) Close(ctx context.Context) error { return nil }
