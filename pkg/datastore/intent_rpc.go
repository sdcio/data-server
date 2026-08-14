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

package datastore

import (
	"context"
	"errors"
	"fmt"

	"github.com/beevik/etree"

	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api/adapter"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

var ErrIntentNotFound = errors.New("intent not found")

func (d *Datastore) applyIntent(ctx context.Context, source targettypes.TargetSource) (*sdcpb.SetDataResponse, error) {
	log := logf.FromContext(ctx)
	var err error

	var rsp *sdcpb.SetDataResponse

	// send set request only if there are updates and/or deletes
	if containsChanges, _ := source.ContainsChanges(ctx); !containsChanges {
		return &sdcpb.SetDataResponse{}, nil
	}

	if d.sbi == nil {
		return nil, fmt.Errorf("%s is not connected", d.config.Name)
	}

	rsp, err = d.sbi.Set(ctx, source)
	if err != nil {
		return nil, err
	}
	log.V(logf.VDebug).Info("got SetResponse from SBI", "raw-response", utils.FormatProtoJSON(rsp))

	return rsp, nil
}

func (d *Datastore) GetIntent(ctx context.Context, intentName string, exposeSensitive bool) (GetIntentResponse, error) {
	// serve running from synctree; sensitive paths are the cross-intent union
	// (running has no own markers — its values may have been echoed back by
	// the device verbatim, so any intent's classification must apply).
	if intentName == consts.RunningIntentName {
		d.syncTreeMutex.RLock()
		defer d.syncTreeMutex.RUnlock()
		err := d.syncTree.FinishInsertionPhase(ctx)
		if err != nil {
			return nil, err
		}

		result := &adapter.IntentResponseAdapter{
			Entry:           d.syncTree.Entry,
			IntentName:      consts.RunningIntentName,
			Priority:        consts.RunningValuesPrio,
			Orphan:          false,
			NonRevertive:    false,
			ExplicitDeletes: nil,
			RenderOpts: ops.RenderOpts{
				IncludeSensitive: exposeSensitive,
				SensitivePathSet: d.sensitivePathIndex,
			},
		}

		return result, nil
	}

	// For a regular intent GET, sensitive-path redaction is scoped to that
	// intent's own markers only.  Another intent's classification does not
	// affect how this intent's data is presented (see ADR 0004).
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, d.taskPool))
	if err != nil {
		return nil, err
	}

	tp, err := d.cacheClient.IntentGet(ctx, intentName)
	if err != nil {
		return nil, err
	}
	protoImporter := proto.NewProtoTreeImporter(tp)

	_, err = root.ImportConfig(ctx, nil, protoImporter, types.NewUpdateInsertFlags(), d.taskPool)
	if err != nil {
		return nil, err
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}

	intentSensitivePaths := types.NewSensitivePathIndex()
	intentSensitivePaths.Add(tp.GetSensitivePaths()...)

	result := &adapter.IntentResponseAdapter{
		Entry:           root.Entry,
		IntentName:      tp.GetIntentName(),
		Priority:        tp.GetPriority(),
		Orphan:          tp.GetOrphan(),
		NonRevertive:    tp.GetNonRevertive(),
		ExplicitDeletes: tp.GetExplicitDeletes(),
		RenderOpts: ops.RenderOpts{
			IncludeSensitive: exposeSensitive,
			SensitivePathSet: intentSensitivePaths,
		},
	}
	return result, nil
}

type GetIntentResponse interface {
	GetIntentName() string
	GetPriority() int32
	IsOrphan() bool
	IsNonRevertive() bool
	GetExplicitDeletes() []*sdcpb.Path
	// ToJson returns the Tree contained structure as JSON
	// use e.g. json.MarshalIndent() on the returned struct
	ToJson(ctx context.Context) (any, error)
	// ToJsonIETF returns the Tree contained structure as JSON_IETF
	// use e.g. json.MarshalIndent() on the returned struct
	ToJsonIETF(ctx context.Context) (any, error)
	ToXML(ctx context.Context) (*etree.Document, error)
	ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error)
	ToXPath(ctx context.Context) (*sdcpb.PathValues, error)
}
