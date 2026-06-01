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

// Package materialize converts a validated tree entry into a SouthboundSetPlan
// suitable for a specific SBI driver, separating encoding decisions from transport.
package materialize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	gnmiutils "github.com/sdcio/data-server/pkg/datastore/target/gnmi/utils"
	"github.com/sdcio/data-server/pkg/datastore/target/gnmi/permodule"
	targettypes "github.com/sdcio/data-server/pkg/datastore/target/types"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// BuildPlan encodes the tree entry into a SouthboundSetPlan for the given SBI.
// If replace is true the plan carries replace semantics (root-level delete for
// gNMI, nc:operation="replace" on the root element for NETCONF).
//
// scb is the schema client bound to this datastore; it is only consumed when
// SBI type is gNMI, the plan uses per-module JSON encoding (see below), and
// merge-delete paths need YANG module origins resolved. It may be nil for
// PROTO, NETCONF, and generic single-root gNMI paths.
//
// Supported SBI types:
//   - "gnmi"  with empty DeviceProfile  →  GnmiSetPlan (single root update)
//   - "gnmi"  with DeviceProfile "cisco-ios-xr" + json/json_ietf  →  GnmiSetPlan (per YANG module via permodule)
//   - "gnmi"  with DeviceProfile "cisco-ios-xr" + proto  →  GnmiSetPlan (generic, single root update)
//   - "netconf" with empty DeviceProfile  →  NetconfSetPlan
//   - "netconf" with DeviceProfile "cisco-ios-xr"  →  NetconfSetPlan (profile accepted; no IOS-XR-specific shaping yet)
func BuildPlan(ctx context.Context, scb schemaClient.SchemaClientBound, sbi *config.SBI, entry api.Entry, replace bool) (targettypes.SouthboundSetPlan, error) {
	if sbi == nil {
		// No SBI configuration present (e.g. noop target in tests); return an
		// empty plan so the driver can decide what to do with it.
		return targettypes.SouthboundSetPlan{}, nil
	}
	if sbi.Type == "gnmi" && sbi.IsCiscoIOSXR() {
		encoding := gnmi.Encoding(gnmiutils.ParseGnmiEncoding(sbi.GnmiOptions.Encoding))
		switch encoding {
		case gnmi.Encoding_JSON, gnmi.Encoding_JSON_IETF:
			plan, err := permodule.Encode(ctx, scb, entry, encoding, replace)
			if err != nil {
				return targettypes.SouthboundSetPlan{}, err
			}
			return targettypes.SouthboundSetPlan{Gnmi: plan}, nil
		}
		// proto (and any other encoding) falls through to the generic gNMI path.
	}

	switch sbi.Type {
	case "gnmi":
		return buildGnmiPlan(ctx, sbi, entry, replace)
	case "netconf":
		return buildNetconfPlan(ctx, sbi, entry, replace)
	default:
		return targettypes.SouthboundSetPlan{}, fmt.Errorf("materialize: unknown SBI type: %q", sbi.Type)
	}
}

// buildGnmiPlan builds a GnmiSetPlan from the tree entry.
// Encoding is selected based on sbi.GnmiOptions.Encoding:
//   - JSON       → whole tree serialised as a single root-level JSON update
//   - JSON_IETF  → whole tree serialised as a single root-level JSON_IETF update
//   - PROTO (default) → individual leaf-level updates via ToProtoUpdates
func buildGnmiPlan(ctx context.Context, sbi *config.SBI, entry api.Entry, replace bool) (targettypes.SouthboundSetPlan, error) {
	encoding := gnmi.Encoding(gnmiutils.ParseGnmiEncoding(sbi.GnmiOptions.Encoding))

	updates, err := buildGnmiUpdates(ctx, entry, encoding)
	if err != nil {
		return targettypes.SouthboundSetPlan{}, err
	}

	deletes, err := buildGnmiDeletes(ctx, entry, replace)
	if err != nil {
		return targettypes.SouthboundSetPlan{}, err
	}

	return targettypes.SouthboundSetPlan{
		Gnmi: &targettypes.GnmiSetPlan{
			Updates: updates,
			Deletes: deletes,
		},
	}, nil
}

// buildGnmiUpdates returns the update list for a gNMI Set request.
// For JSON/JSON_IETF encoding the whole tree is serialised as a single
// root-level update. For PROTO encoding individual leaf updates are returned.
func buildGnmiUpdates(ctx context.Context, entry api.Entry, encoding gnmi.Encoding) ([]*sdcpb.Update, error) {
	switch encoding {
	case gnmi.Encoding_JSON:
		data, err := ops.ToJson(ctx, entry, true)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("materialize: marshal JSON: %w", err)
		}
		return []*sdcpb.Update{{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: b}},
		}}, nil

	case gnmi.Encoding_JSON_IETF:
		data, err := ops.ToJsonIETF(ctx, entry, true)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("materialize: marshal JSON_IETF: %w", err)
		}
		return []*sdcpb.Update{{
			Path:  &sdcpb.Path{Elem: []*sdcpb.PathElem{}},
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: b}},
		}}, nil

	default:
		// PROTO: per-leaf updates
		return ops.ToProtoUpdates(ctx, entry, true)
	}
}

// buildGnmiDeletes returns the delete list for a gNMI Set request.
// For replace semantics a single root-path delete is returned so the driver
// replaces the full device configuration. For merge semantics the tree's own
// computed deletes are used.
func buildGnmiDeletes(ctx context.Context, entry api.Entry, replace bool) ([]*sdcpb.Path, error) {
	if replace {
		return []*sdcpb.Path{{Elem: []*sdcpb.PathElem{}}}, nil
	}
	return ops.ToProtoDeletes(ctx, entry)
}

// buildNetconfPlan builds a NetconfSetPlan from the tree entry.
// For replace semantics nc:operation="replace" is stamped on the document root.
func buildNetconfPlan(ctx context.Context, sbi *config.SBI, entry api.Entry, replace bool) (targettypes.SouthboundSetPlan, error) {
	opts := sbi.NetconfOptions

	doc, err := ops.ToXML(ctx, entry, true,
		opts.IncludeNS,
		opts.OperationWithNamespace,
		opts.UseOperationRemove,
	)
	if err != nil {
		return targettypes.SouthboundSetPlan{}, err
	}

	if replace {
		utils.AddXMLOperation(&doc.Element, utils.XMLOperationReplace,
			opts.OperationWithNamespace,
			opts.UseOperationRemove,
		)
	}

	return targettypes.SouthboundSetPlan{
		Netconf: &targettypes.NetconfSetPlan{Doc: doc},
	}, nil
}
