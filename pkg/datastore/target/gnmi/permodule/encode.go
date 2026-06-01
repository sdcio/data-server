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

// Package permodule encodes gNMI Set plans with one Update per YANG module at
// the datastore root: Path.Origin is the module name and Path.Elem[0] is the
// module's top-level container.  The encoder walks direct children of the root
// tree entry, groups them by YANG module, and serialises each group into one
// sdcpb.Update.
//
// Some targets require this layout (for example Cisco IOS-XR with JSON
// encodings); the package name reflects the encoding shape, not a specific
// device profile constant.
package permodule

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openconfig/gnmi/proto/gnmi"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/api"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// Encode builds a GnmiSetPlan from the tree entry using per-YANG-module grouping.
//
//   - replace=false (merge): only new-or-updated leaves per module are included;
//     modules with no changed leaves produce no Update.  Merge deletes are
//     included with Path.Origin resolved to the owning YANG module.
//   - replace=true: for every module present in the tree, a module-root delete
//     (with Path.Origin) is emitted followed by the module's full update, all
//     in a single GnmiSetPlan so the device applies them atomically.
//
// scb is used only for resolving the YANG module name of path-only delete
// entries (arising from choice-case changes); it may be nil if the tree is
// known to have no such entries.
func Encode(
	ctx context.Context,
	scb schemaClient.SchemaClientBound,
	entry api.Entry,
	encoding gnmi.Encoding,
	replace bool,
) (*targettypes.GnmiSetPlan, error) {
	if replace {
		return encodeReplace(ctx, entry, encoding)
	}
	return encodeMerge(ctx, scb, entry, encoding)
}

// moduleGroup collects all direct children of the root that belong to the
// same YANG module.
type moduleGroup struct {
	moduleName string
	children   []api.Entry
}

// groupByModule walks the direct children of entry and returns one
// moduleGroup per distinct YANG module name, preserving insertion order.
func groupByModule(entry api.Entry) []moduleGroup {
	var order []string
	groups := map[string]*moduleGroup{}

	for _, child := range entry.GetChilds(treetypes.DescendMethodActiveChilds) {
		schema := child.GetSchema()
		if schema == nil {
			continue
		}
		mod := utils.GetSchemaElemModuleName(schema)
		if mod == "" {
			continue
		}
		if _, exists := groups[mod]; !exists {
			groups[mod] = &moduleGroup{moduleName: mod}
			order = append(order, mod)
		}
		groups[mod].children = append(groups[mod].children, child)
	}

	result := make([]moduleGroup, 0, len(order))
	for _, mod := range order {
		result = append(result, *groups[mod])
	}
	return result
}

// serializeChild serialises a single child entry to a JSON or JSON_IETF blob.
// Returns nil, nil when the result is empty (e.g. no new-or-updated leaves in
// merge mode).
func serializeChild(ctx context.Context, child api.Entry, encoding gnmi.Encoding, onlyNewOrUpdated bool) ([]byte, error) {
	var data any
	var err error

	switch encoding {
	case gnmi.Encoding_JSON_IETF:
		data, err = ops.ToJsonIETF(ctx, child, onlyNewOrUpdated)
	default:
		data, err = ops.ToJson(ctx, child, onlyNewOrUpdated)
	}
	if err != nil {
		return nil, fmt.Errorf("permodule: serialise %s: %w", child.PathName(), err)
	}
	if data == nil {
		return nil, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("permodule: marshal %s: %w", child.PathName(), err)
	}
	return b, nil
}

// makeTypedValue wraps a JSON byte slice in the appropriate TypedValue variant.
func makeTypedValue(b []byte, encoding gnmi.Encoding) *sdcpb.TypedValue {
	if encoding == gnmi.Encoding_JSON_IETF {
		return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: b}}
	}
	return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: b}}
}

// moduleRootPath constructs the gNMI path for a module root container.
func moduleRootPath(moduleName, containerName string) *sdcpb.Path {
	return &sdcpb.Path{
		Origin: moduleName,
		Elem:   []*sdcpb.PathElem{{Name: containerName}},
	}
}

// encodeMerge builds a GnmiSetPlan for a merge (non-replace) transaction.
func encodeMerge(
	ctx context.Context,
	scb schemaClient.SchemaClientBound,
	entry api.Entry,
	encoding gnmi.Encoding,
) (*targettypes.GnmiSetPlan, error) {
	groups := groupByModule(entry)

	plan := &targettypes.GnmiSetPlan{}

	for _, g := range groups {
		for _, child := range g.children {
			b, err := serializeChild(ctx, child, encoding, true)
			if err != nil {
				return nil, err
			}
			if b == nil {
				// No new-or-updated leaves in this module — omit.
				continue
			}
			plan.Updates = append(plan.Updates, &sdcpb.Update{
				Path:  moduleRootPath(g.moduleName, child.PathName()),
				Value: makeTypedValue(b, encoding),
			})
		}
	}

	// Collect merge deletes with Origin resolved.
	deletes, err := mergeDeletes(ctx, scb, entry)
	if err != nil {
		return nil, err
	}
	plan.Deletes = deletes

	return plan, nil
}

// encodeReplace builds a GnmiSetPlan for a replace transaction.
// For each module present in the tree it emits a module-root delete
// (with Path.Origin) followed by the full module update.
func encodeReplace(
	ctx context.Context,
	entry api.Entry,
	encoding gnmi.Encoding,
) (*targettypes.GnmiSetPlan, error) {
	groups := groupByModule(entry)

	plan := &targettypes.GnmiSetPlan{}

	for _, g := range groups {
		for _, child := range g.children {
			b, err := serializeChild(ctx, child, encoding, false)
			if err != nil {
				return nil, err
			}
			rootPath := moduleRootPath(g.moduleName, child.PathName())

			// Per-module root delete before the full update.
			plan.Deletes = append(plan.Deletes, rootPath)
			if b != nil {
				plan.Updates = append(plan.Updates, &sdcpb.Update{
					Path:  rootPath,
					Value: makeTypedValue(b, encoding),
				})
			}
		}
	}

	return plan, nil
}

// mergeDeletes collects all explicit delete paths from the tree and resolves
// the YANG module origin for each one.
func mergeDeletes(
	ctx context.Context,
	scb schemaClient.SchemaClientBound,
	entry api.Entry,
) ([]*sdcpb.Path, error) {
	rawDeletes, err := ops.GetDeletes(entry, true)
	if err != nil {
		return nil, fmt.Errorf("permodule: collect deletes: %w", err)
	}
	if len(rawDeletes) == 0 {
		return nil, nil
	}

	result := make([]*sdcpb.Path, 0, len(rawDeletes))
	for _, del := range rawDeletes {
		origin, err := resolveDeleteOrigin(ctx, scb, del)
		if err != nil {
			return nil, err
		}
		p := del.SdcpbPath()
		withOrigin := &sdcpb.Path{
			Origin: origin,
			Elem:   p.GetElem(),
		}
		result = append(result, withOrigin)
	}
	return result, nil
}

// resolveDeleteOrigin determines the YANG module name for a delete entry.
//
//   - For schema-attached entries (api.Entry with GetSchema() != nil) the
//     module is read directly via GetSchemaElemModuleName.
//   - For path-only entries (DeleteEntryImpl, no schema attached) the module
//     is resolved by fetching the schema for the first path element via scb.
func resolveDeleteOrigin(ctx context.Context, scb schemaClient.SchemaClientBound, del treetypes.DeleteEntry) (string, error) {
	// Schema-attached: the entry already carries the schema element.
	if e, ok := del.(api.Entry); ok && e.GetSchema() != nil {
		return utils.GetSchemaElemModuleName(e.GetSchema()), nil
	}

	// Path-only: look up via schema client.
	path := del.SdcpbPath()
	if len(path.GetElem()) == 0 || scb == nil {
		return "", nil
	}
	firstElem := &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: path.GetElem()[0].GetName()}}}
	rsp, err := scb.GetSchemaSdcpbPath(ctx, firstElem)
	if err != nil {
		return "", fmt.Errorf("permodule: schema lookup for delete path %v: %w", path, err)
	}
	return utils.GetSchemaElemModuleName(rsp.GetSchema()), nil
}
