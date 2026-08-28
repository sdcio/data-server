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

// Package parentbound encodes gNMI Set plans with parent-bound JSON payloads
// suitable for SONiC translib: plain containers batch changed sibling leaves
// into one Update at the container path; keyed list rows emit one Update per
// touched row at the list-instance path with a full-row array wrap.
package parentbound

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/api"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const sonicOrigin = "sonic_yang"

// Encode builds a GnmiSetPlan from the tree entry using parent-bound grouping
// for SONiC translib. Only merge (Update) semantics are supported; the replace
// flag is ignored and Replace-equivalent operations are never emitted.
//
// scb is reserved for future delete-path schema lookups; unused today.
func Encode(
	ctx context.Context,
	scb schemaClient.SchemaClientBound,
	entry api.Entry,
	replace bool,
) (*targettypes.GnmiSetPlan, error) {
	_ = scb
	_ = replace

	changedLeaves := collectChangedLeaves(entry)
	groups := groupByParent(changedLeaves)

	plan := &targettypes.GnmiSetPlan{}

	for _, g := range groups {
		upd, err := buildUpdate(ctx, g)
		if err != nil {
			return nil, err
		}
		if upd != nil {
			plan.Updates = append(plan.Updates, upd)
		}
	}

	deletes, err := ops.ToProtoDeletes(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("parentbound: collect deletes: %w", err)
	}
	plan.Deletes = deletes

	if len(plan.Updates) == 0 && len(plan.Deletes) == 0 {
		return nil, nil
	}
	return plan, nil
}

type parentGroup struct {
	target  api.Entry
	listRow bool
}

func groupByParent(leaves []api.Entry) []parentGroup {
	seen := map[api.Entry]struct{}{}
	order := make([]parentGroup, 0, len(leaves))

	for _, leaf := range leaves {
		target, listRow := groupingTarget(leaf)
		if target == nil {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		order = append(order, parentGroup{target: target, listRow: listRow})
	}
	return order
}

func groupingTarget(leaf api.Entry) (api.Entry, bool) {
	parent := leaf.GetParent()
	if parent == nil || parent.IsRoot() {
		return nil, false
	}
	if parent.GetSchema() == nil {
		ancestor, level := ops.GetFirstAncestorWithSchema(parent)
		keys := ops.GetSchemaKeys(ancestor)
		if len(keys) > 0 && level == len(keys) {
			return parent, true
		}
		return parent, false
	}
	switch parent.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		if len(ops.GetSchemaKeys(parent)) > 0 {
			return parent, true
		}
		return parent, false
	default:
		return parent, false
	}
}

func collectChangedLeaves(e api.Entry) []api.Entry {
	var leaves []api.Entry
	collectChangedLeavesRecursive(e, &leaves)
	return leaves
}

func collectChangedLeavesRecursive(e api.Entry, out *[]api.Entry) {
	if isChangedLeaf(e) {
		*out = append(*out, e)
	}
	for _, child := range e.GetChilds(treetypes.DescendMethodActiveChilds) {
		collectChangedLeavesRecursive(child, out)
	}
}

func isChangedLeaf(e api.Entry) bool {
	if e.GetSchema() == nil {
		return false
	}
	switch e.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Field, *sdcpb.SchemaElem_Leaflist:
		le := e.GetLeafVariants().GetHighestPrecedence(true, false, false)
		return le != nil && (le.IsNew || le.IsUpdated)
	default:
		return false
	}
}

func buildUpdate(ctx context.Context, g parentGroup) (*sdcpb.Update, error) {
	var body any
	var err error

	if g.listRow {
		body, err = serializeListRowUpdate(ctx, g.target)
	} else {
		body, err = ops.ToJsonIETF(ctx, g.target, true)
	}
	if err != nil {
		return nil, fmt.Errorf("parentbound: serialise %s: %w", g.target.PathName(), err)
	}
	if body == nil {
		return nil, nil
	}

	stripped := stripRFC7951Prefixes(body)
	b, err := json.Marshal(stripped)
	if err != nil {
		return nil, fmt.Errorf("parentbound: marshal %s: %w", g.target.PathName(), err)
	}

	path := g.target.SdcpbPath().DeepCopy()
	path.Origin = sonicOrigin

	return &sdcpb.Update{
		Path:  path,
		Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: b}},
	}, nil
}

func serializeListRowUpdate(ctx context.Context, listInstance api.Entry) (any, error) {
	row, err := ops.ToJsonIETF(ctx, listInstance, false)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	listContainer := listInstance.GetParent()
	if listContainer == nil {
		return nil, fmt.Errorf("list instance %s has no parent", listInstance.PathName())
	}

	wrapKey := jsonIETFKey(listContainer)
	return map[string]any{
		wrapKey: []any{row},
	}, nil
}

func jsonIETFKey(e api.Entry) string {
	key := e.PathName()
	ancestor, _ := ops.GetFirstAncestorWithSchema(e)
	if ancestor == nil || e.GetSchema() == nil {
		return key
	}
	if utils.GetSchemaElemModuleName(e.GetSchema()) == utils.GetSchemaElemModuleName(ancestor.GetSchema()) {
		return key
	}
	return fmt.Sprintf("%s:%s", utils.GetSchemaElemModuleName(e.GetSchema()), key)
}

func stripRFC7951Prefixes(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[stripKeyPrefix(k)] = stripRFC7951Prefixes(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = stripRFC7951Prefixes(val)
		}
		return out
	default:
		return v
	}
}

func stripKeyPrefix(k string) string {
	if idx := strings.LastIndex(k, ":"); idx >= 0 {
		return k[idx+1:]
	}
	return k
}
