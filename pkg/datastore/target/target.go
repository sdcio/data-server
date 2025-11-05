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
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/grpc"

	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi"
	"github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/pool"
)

const (
	targetTypeNOOP    = "noop"
	targetTypeNETCONF = "netconf"
	targetTypeGNMI    = "gnmi"
)

type Target interface {
	Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error)
	Set(ctx context.Context, source types.TargetSource) (*sdcpb.SetDataResponse, error)
	AddSyncs(ctx context.Context, sps ...*config.SyncProtocol) error
	Status() *types.TargetStatus
	Close() error
}

func New(ctx context.Context, name string, cfg *config.SBI, schemaClient schemaClient.SchemaClientBound, runningStore types.RunningStore, syncConfigs []*config.SyncProtocol, taskpoolFactory pool.VirtualPoolFactory, opts ...grpc.DialOption) (Target, error) {
	var t Target
	var err error

	targetContext := context.Background()

	switch cfg.Type {
	case targetTypeGNMI:
		t, err = gnmi.NewTarget(targetContext, name, cfg, runningStore, schemaClient, taskpoolFactory, opts...)
		if err != nil {
			return nil, err
		}
	// case targetTypeNETCONF:
	// return newNCTarget(ctx, name, cfg, schemaClient)
	// case targetTypeNOOP, "":
	// 	return newNoopTarget(ctx, name)
	default:
		return nil, fmt.Errorf("unknown DS target type %q", cfg.Type)

	}

	err = t.AddSyncs(targetContext, syncConfigs...)
	if err != nil {
		return nil, err
	}

	return t, nil
}
