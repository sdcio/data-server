# IOS-XR granular gNMI JSON must be built from `api.Entry`, not from serialized JSON

For the `cisco-ios-xr` device profile, gNMI `Update` messages (one per YANG module) must be constructed by walking the schema-attached `api.Entry` tree — not by inspecting keys in a post-serialized JSON blob.

## Why

IOS-XR native YANG requires each gNMI `Update` in a `SetRequest` to be scoped to a single YANG module: the `Path.origin` field must be set to the module name (e.g. `Cisco-IOS-XR-ip-static-cfg`) and the path element must be the module's root container. Grouping data by module requires knowing, for each tree node, which YANG module it belongs to. This information is available in `api.Entry` via `GetSchemaElemModuleName` — it is not reliably recoverable by parsing JSON key prefixes after serialization.

The post-hoc JSON approach (as attempted in PR #303) only works for the first two levels of nesting, silently drops deeper module changes, and requires maintaining external state (`nsMap`) that is not thread-safe and does not survive restarts.

## Considered options

**Rejected — post-hoc JSON key parsing:** Split a monolithic `json_ietf_val` blob by inspecting `"module:key"` prefixes at the top two levels. Fails for deeper nesting, fragile, requires mutable global state.

**Accepted — walk `api.Entry`:** Group direct children of the root entry by `GetSchemaElemModuleName(child.GetSchema())`. For each module group, serialize the subtree with `ToJsonIETF` scoped to that group and emit one `sdcpb.Update` with `Path.origin` set to the module name. All updates go in a single `SetRequest` for atomicity (splitting into multiple `SetRequest` RPCs would break transactional correctness).
