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
	"github.com/sdcio/data-server/pkg/tree/importer/proto"
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

func (d *Datastore) GetIntent(ctx context.Context, intentName string) (GetIntentResponse, error) {
	// serve running from synctree
	if intentName == tree.RunningIntentName {
		d.syncTreeMutex.RLock()
		defer d.syncTreeMutex.RUnlock()
		err := d.syncTree.FinishInsertionPhase(ctx)
		if err != nil {
			return nil, err
		}

		return newTreeRootToGetIntentResponse(d.syncTree), nil
	}

	// otherwise consult cache
	root, err := tree.NewTreeRoot(ctx, tree.NewTreeContext(d.schemaClient, intentName))
	if err != nil {
		return nil, err
	}

	tp, err := d.cacheClient.IntentGet(ctx, intentName)
	if err != nil {
		return nil, err
	}
	protoImporter := proto.NewProtoTreeImporter(tp)

	err = root.ImportConfig(ctx, nil, protoImporter, tp.GetIntentName(), tp.GetPriority(), tp.GetNonRevertive(), types.NewUpdateInsertFlags())
	if err != nil {
		return nil, err
	}

	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		return nil, err
	}
	return newTreeRootToGetIntentResponse(root), nil
}

type GetIntentResponse interface {
	// ToJson returns the Tree contained structure as JSON
	// use e.g. json.MarshalIndent() on the returned struct
	ToJson() (any, error)
	// ToJsonIETF returns the Tree contained structure as JSON_IETF
	// use e.g. json.MarshalIndent() on the returned struct
	ToJsonIETF() (any, error)
	ToXML() (*etree.Document, error)
	ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error)
}

type treeRootToGetIntentResponse struct {
	root *tree.RootEntry
}

func newTreeRootToGetIntentResponse(root *tree.RootEntry) *treeRootToGetIntentResponse {
	return &treeRootToGetIntentResponse{
		root: root,
	}
}

func (t *treeRootToGetIntentResponse) ToJson() (any, error) {
	return t.root.ToJson(false)
}

func (t *treeRootToGetIntentResponse) ToJsonIETF() (any, error) {
	return t.root.ToJsonIETF(false)
}

func (t *treeRootToGetIntentResponse) ToXML() (*etree.Document, error) {
	return t.root.ToXML(false, true, false, false)
}
func (t *treeRootToGetIntentResponse) ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error) {
	return t.root.ToProtoUpdates(ctx, false)
}
