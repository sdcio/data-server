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

package configserver

import (
	"encoding/json"
	"fmt"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/proto"
)

// DocumentFromIntent flattens a tree_persist.Intent — the schema-typed tree
// ops.TreeExport produces for one owner's contribution at apply time — into
// the same flat Document shape LocalConfigReader already returns for
// reads, so LocalConfigWriter.Modify can send it as a ConfigSnapshotService
// wire payload without a second, parallel field-mapping table.
//
// The whole tree collapses into a single ConfigBlob at path "/": each
// TreeElement becomes a JSON object key, with same-named siblings — the
// shape a list container's instances always take in the exported tree,
// since TreeExport names every list instance after its containing list,
// never its key value — grouped into a JSON array. mergeConfigBlobs's
// mergeRootBlob (the read-side counterpart) already treats path "/" as a
// shallow merge of a JSON object's top-level keys, which is exactly this
// shape.
func DocumentFromIntent(target Target, name string, intent *tree_persist.Intent) (*Document, error) {
	value, err := treeElementToJSON(intent.GetRoot())
	if err != nil {
		return nil, fmt.Errorf("flatten intent %q: %w", intent.GetIntentName(), err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal intent %q: %w", intent.GetIntentName(), err)
	}

	return &Document{
		Name:           name,
		Namespace:      target.Namespace,
		Priority:       intent.GetPriority(),
		NonRevertive:   intent.GetNonRevertive(),
		Orphan:         intent.GetOrphan(),
		SensitivePaths: intent.GetSensitivePaths(),
		Config:         []*ConfigBlob{{Path: "/", Value: data}},
	}, nil
}

// treeElementToJSON renders el into the plain `any` shape encoding/json can
// marshal directly: a decoded scalar/array/object for a leaf (off its
// TypedValue), or a nested object for a container — see
// childrenToJSON for how same-named Childs (list instances) group into an
// array.
func treeElementToJSON(el *tree_persist.TreeElement) (any, error) {
	if el == nil {
		return nil, nil
	}
	if len(el.GetChilds()) > 0 {
		return childrenToJSON(el.GetChilds())
	}
	if len(el.GetLeafVariant()) == 0 {
		// A container-shaped element with neither children nor a value of
		// its own, e.g. an empty presence container.
		return map[string]any{}, nil
	}

	tv := &sdcpb.TypedValue{}
	if err := proto.Unmarshal(el.GetLeafVariant(), tv); err != nil {
		return nil, fmt.Errorf("unmarshal leaf %q: %w", el.GetName(), err)
	}
	return utils.GetJsonValue(tv, true)
}

// childrenToJSON groups childs by Name into a JSON object, collecting
// same-named siblings into a JSON array under that key. TreeExport always
// names every instance of a list after the list itself (never the
// instance's key value), so grouping by Name is exactly how list instances
// recombine — no schema access needed to tell "this name is a list" from
// "this name is a plain container": JsonTreeImporter.GetElements already
// treats a bare object and a one-element array under the same key
// identically on the read side, so a single-instance list can stay
// ungrouped without losing information.
func childrenToJSON(childs []*tree_persist.TreeElement) (any, error) {
	order := make([]string, 0, len(childs))
	byName := map[string][]any{}
	for _, c := range childs {
		v, err := treeElementToJSON(c)
		if err != nil {
			return nil, err
		}
		if _, seen := byName[c.GetName()]; !seen {
			order = append(order, c.GetName())
		}
		byName[c.GetName()] = append(byName[c.GetName()], v)
	}

	result := make(map[string]any, len(order))
	for _, name := range order {
		vals := byName[name]
		if len(vals) > 1 {
			result[name] = vals
		} else {
			result[name] = vals[0]
		}
	}
	return result, nil
}
