Status: ready-for-agent

# SONiC (`sonic_yang`/translib) `GnmiSetPlan` device-profile

## Problem Statement

sdcio's generic gNMI Set encoding sends one Update with an empty path and the entire intended JSON tree as `JsonIetfVal`. SONiC's `translib` northbound (the implementation behind gNMI origin `sonic_yang`, reachable today via a custom `telemetry` binary built with `gnmi_translib_write`) does not accept that shape at all:

- It reads **only** `json_ietf_val` — plain `json` or `proto` encodings silently produce an empty payload (single-op Set) or an explicit `InvalidArgument` (bulk Set), and translib then fails with `"Request payload is empty"`.
- For UPDATE/REPLACE it unmarshals the JSON payload onto the **parent of the last path element**, not onto the node the path itself names (`request_binder.go`'s `unMarshall`).
- Its generated (`ocbinds`) Go structs use unprefixed `path:` tags, so RFC 7951 module-prefixed JSON keys (e.g. `sonic-srv6:SRV6_MY_LOCATORS`) are rejected with `"JSON contains unexpected field"`.
- Get/Subscribe against `sonic_yang` also can't be done against a single root (`/`) path the way sdcio does for OpenConfig-style targets — translib registers one app per top-level module path prefix and hard-errors on an empty/root URI (`"Path is empty"`).

None of this can be fixed on the SONiC side for this workstream (translib/`request_binder.go` is explicitly out of scope — no SONiC patches). sdcio needs a southbound-side fix, following the pattern established by the Cisco IOS-XR `device-profile` materialization work ([sdcio/data-server#442](https://github.com/sdcio/data-server/pull/442)): a closed-set `device-profile` on the Target's SBI config that selects a NOS-specific `GnmiSetPlan` encoder, with `gnmi.go` remaining pure transport.

## Solution

Add a new `DeviceProfileSonic` (`"sonic"`) device-profile, mirroring how `DeviceProfileCiscoIOSXR` is wired today:

- A generic (schema-driven, not per-YANG-module) encoder that, for a gNMI target with `device-profile: sonic`, walks the changed-entry tree and produces `GnmiSetPlan` Updates that are correctly parent-bound for translib, with RFC 7951 module prefixes stripped recursively from JSON keys.
- Closed-set config validation that only accepts `JSON_IETF` encoding for this profile (translib cannot consume anything else), rejected at SBI config-load time.
- No changes to `gnmi.go`, `target.New`, or any other generic/type-agnostic infrastructure — dispatch happens entirely inside `materialize.BuildPlan`, the same seam Cisco's `permodule` uses.
- A documented (not code-fixed) limitation for Get/Subscribe: translib needs one path per top-level module, so operators must list per-module paths in `SyncConfig.Paths` rather than relying on the usual `/` convention. No schema-driven path-expansion code is built in this iteration.
- A companion protobuf change on `sdc-protos`' still-open `deviceprofile` branch ([sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)), adding `DEVICE_PROFILE_SONIC = 2` to the `DeviceProfile` enum, so the profile is exposed on the Target CR/gRPC API the same way Cisco's is — not left as a config-only value.

## User Stories

1. As an operator managing a SONiC device (`sonic_yang` origin, custom translib-write-enabled `telemetry` binary) through sdcio, I want to set `device-profile: sonic` on the Target's SBI config, so that Set operations against that device succeed instead of failing with translib's "Request payload is empty"/"JSON contains unexpected field" errors.
2. As an operator, I want a Set that changes a single leaf under an existing container to result in one gNMI Update whose path is the leaf's path and whose JSON value is `{leafName: value}` wrapped one level up (bound to the leaf's parent), so that translib's parent-bind unmarshal accepts it.
3. As an operator, I want a Set that creates or modifies a row in a SONiC keyed list (e.g. `SRV6_MY_LOCATORS_LIST[locator_name=MAIN]`) to result in one gNMI Update at the list-instance path, with a JSON value that is the array-wrapped full row (`{"SRV6_MY_LOCATORS_LIST":[{...}]}`), so that a brand-new row can be created reliably without depending on unverified partial-leaf list-row creation behavior.
4. As an operator, I want multiple leaves that changed together under the same plain (non-list) container to be batched into a single Update carrying all of them as one JSON object, so that fewer round trips are needed without risking sibling data loss (verified: translib UPDATE only writes JSON-present fields to Redis via `HMSET`; it never wipes unmentioned fields — that destructive behavior is REPLACE-only and this profile never emits gNMI `Replace`, only `Update`).
5. As an operator, I want JSON body keys emitted for this profile to have their RFC 7951 module prefixes stripped recursively (at every depth a prefix appears, not just the top key), so that translib's unprefixed `ocbinds` struct tags accept the payload regardless of how deep an augmentation/module boundary sits in the tree.
6. As an operator, I want a delete against a `sonic_yang` path to work unmodified (no encoder involvement needed), so that I don't have to think differently about deletes vs. updates for this profile — translib DELETE is path-only with no JSON body requirement.
7. As an operator, I want multiple Set changes destined for the same device to be batched into one gNMI `SetRequest` where possible, so that they land as a single atomic `translib.Bulk` transaction on the SONiC side rather than N independent non-atomic writes.
8. As an operator configuring a Target with `device-profile: sonic`, I want the config to be rejected at load time if I configure any encoding other than `JSON_IETF` for that SBI, so that I get a fast, clear error instead of a confusing runtime Set failure.
9. As a maintainer, I want the sonic device-profile's dispatch condition (`sbi.Type == "gnmi" && sbi.IsSonic()`) to live entirely inside `materialize.BuildPlan`, with the actual encoding logic in its own package (sibling to `permodule`), so that no NOS-specific `if`/`switch` branches leak into `gnmi.go`, `target.New`, or other generic infrastructure (per the repo's "avoid tight coupling on target types" rule).
10. As an operator setting up sync/subscribe against a SONiC device, I want documentation telling me that a single root (`/`) sync path will fail, and that I need to list one path per top-level `sonic_yang` module in `SyncConfig.Paths` instead, so that I don't have to rediscover this the hard way.
11. As a maintainer, I want the `DeviceProfile` enum exposed on the gRPC/Target-CR layer (not just in local YAML config), matching how `DEVICE_PROFILE_CISCO_IOS_XR` is exposed, so the sonic profile can eventually be set via the Target API/CRD, not only via static SBI config files.

## Implementation Decisions

### Device profile

- New constant `DeviceProfileSonic DeviceProfile = "sonic"` in `pkg/config`, alongside the existing `DeviceProfileNone`/`DeviceProfileCiscoIOSXR`.
- New predicate `(*SBI).IsSonic() bool`, mirroring `IsCiscoIOSXR()`.
- Closed-set validation extended to accept `"sonic"` as a valid `device-profile` value.
- Additional validation specific to this profile: when `device-profile == "sonic"`, the SBI's configured `GnmiOptions.Encoding` (or equivalent) must be `JSON_IETF` only — reject at config validation time (not just at `materialize.BuildPlan` runtime) if any other encoding is configured. `BuildPlan` may additionally assert this defensively, but config-time validation is the primary guard.
- Companion protobuf: `sdc-protos`' `data.proto` `enum DeviceProfile` gains `DEVICE_PROFILE_SONIC = 2` (on the existing open `deviceprofile` branch, not `main` directly — that branch is what `sdc-protos#120`/data-server's `ciscoiosxrd2` branch depend on). `pkg/server/datastore.go`'s `sdcpbDeviceProfileToConfig`/`configDeviceProfileToSdcpb` conversion switches gain the corresponding case, matching the existing Cisco IOS-XR mapping.

### Dispatch

- `materialize.BuildPlan` gains a branch: `sbi.Type == "gnmi" && sbi.IsSonic()` → call the new encoder package's `Encode(...)`, mirroring exactly how the existing `sbi.IsCiscoIOSXR()` branch calls `permodule.Encode(...)`.
- No fallback to the generic gNMI plan for non-`JSON_IETF` encodings on this profile (unlike Cisco's IOS-XR+proto fallback, which works because IOS-XR is a real proto-native gNMI target) — this is enforced primarily by the config-time validation above, not by a runtime fallback/error branch doing double duty.
- `target.New` is unaffected — device-profile selection happens only inside `materialize.BuildPlan`, same as Cisco's.

### Encoder (new package, sibling to `pkg/datastore/target/gnmi/permodule/`; exact package name deferred to implementation/codebase-design — do not block on bikeshedding it)

Entry point: `Encode(ctx, scb schemaClient.SchemaClientBound, entry api.Entry, replace bool) (*targettypes.GnmiSetPlan, error)`, matching `permodule.Encode`'s signature shape.

Algorithm — schema-driven, not per-YANG-module (no hardcoded knowledge of any specific `sonic-*.yang` file):

1. Walk the changed-entry tree (`entry.GetChilds`, `entry.GetLeafVariants()`), and for each changed leaf, find its immediate parent node.
2. Classify the parent's schema via the existing `GetSchema().GetSchema()` type-switch idiom (already used in `pkg/tree/ops/json.go`/`xml.go`) plus `ops.GetSchemaKeys(parent)`:
   - **Keyed list** (`len(GetSchemaKeys(parent)) > 0`): group by the list-instance path (all leaves under the same row). Emit ONE Update per touched row, at the list-instance's `sdcpb.Path` (with its key), whose JSON value is the array-wrapped **full row** — i.e. include the row's complete current field set, not just the leaves that changed in this operation. This sidesteps the untested question of whether translib's `GetOrCreateNode` reliably creates a brand-new list row from a single partial-leaf Update; the full-row-wrap shape is the one verified working (including for brand-new rows) against the live lab.
   - **Plain (non-list) container**: group all directly-changed sibling leaves under that container into ONE Update at the container's path, whose JSON value is a plain object containing just those changed leaves (siblings not mentioned are safe — see Testing Decisions/verified facts below).
3. Recurse: nested containers/lists produce their own Updates following the same rule at their own level: this is NOT a per-module (whole-subtree) grouping like `permodule`'s — it groups by immediate parent, at whatever depth the change occurred, which may be much finer than "one Update per module."
4. For every produced Update, force `Path.Origin = "sonic_yang"` regardless of what origin the source entry carried (mirrors `permodule.Encode` always setting `Path.Origin = moduleName`).
5. Serialize the JSON value for each Update, then strip RFC 7951 module prefixes (`module:name` → `name`) **recursively at every depth** in the JSON body — not just the top-level wrap key — since translib's `ocbinds` structs use unprefixed `path:` tags throughout, including past nested augmentation/module boundaries. (Path-side elements do NOT need prefix stripping — translib's `request_binder.go` already strips `module:` prefixes from every path element itself, regardless of position.)
6. Deletes: pass through unmodified as path-only deletes (`GnmiSetPlan.Deletes`) — no JSON shaping needed; translib DELETE takes no payload at all (confirmed: `unMarshall` returns before any payload unmarshal for opcode DELETE).
7. All produced Updates for one `BuildPlan` call ride in the same `GnmiSetPlan`, which `gnmi.go`'s `Set` already sends as a single `gnmi.SetRequest` — no change needed there. sonic-gnmi's `TranslClient.Set` automatically routes any `SetRequest` with more than one total op (deletes+replaces+updates) through `TranslProcessBulk` → `translib.Bulk`, a single atomic Redis transaction; a single-op `SetRequest` uses the non-bulk per-call path. Either way, this encoder always uses the gNMI `Update` field only (never `Replace`) for its Updates, exactly like `gnmi.go`'s existing `Set` already does for all `GnmiSetPlan.Updates` — so translib's destructive REPLACE-only sibling-delete behavior (`processDeleteForReplace`) is never triggered by this profile.

### Get/Subscribe (documented limitation, no code changes this iteration)

- translib has an effective one-URI-→-one-app model; a root (`/`) Get/Subscribe hard-errors with `"Path is empty"`.
- sdcio's generic Get/Subscribe/Sync code sends whatever paths are configured verbatim in one RPC, with no schema-driven per-module expansion anywhere in the codebase — this is unchanged by this work.
- Workaround for v1: operators must list one path per top-level `sonic_yang` module container explicitly in the Target's `SyncConfig.Paths` (already a free-text string list, no code change required for this to work). Ad-hoc single-shot Get calls using a root path are unsupported for this profile until a future iteration adds automatic schema-driven path expansion (out of scope here).
- translib's GET JSON response shape was independently verified to already be standard RFC 7951 (module prefix only at namespace-crossing points, same convention every other RFC-7951-emitting device produces) — this is expected to already be parsed correctly by sdcio's existing generic Get path with zero changes, and is the structural mirror of the recursive-prefix-stripping done on the Set side.

## Testing Decisions

Three seams, all mirroring existing prior art for the Cisco IOS-XR device-profile (confirmed with the requester before writing this spec — no new seams, no changes needed to `gnmi.go`):

1. **Encoder package** (`Encode(...)`): unit tests against fixture trees, table-driven, same style as `pkg/datastore/target/gnmi/permodule/encode_test.go`. Cases to cover at minimum:
   - Single leaf changed under an existing plain container → one Update, path = leaf's parent... (path/value shape as decided above), origin forced to `sonic_yang`.
   - Multiple sibling leaves changed together under the same plain container → one Update bundling all of them, not N separate Updates.
   - New row created in a keyed list → one Update at the list-instance path, full-row array-wrapped JSON body.
   - Existing row's field changed in a keyed list → same full-row-wrap shape (not a partial leaf-only Update), to keep the rule uniform and avoid relying on untested partial-row behavior.
   - Nested structure (module → container → list) → confirms the "group by immediate parent, recurse independently at each level" rule rather than one giant per-module blob.
   - RFC 7951-prefixed input at multiple depths (including a simulated cross-module augmentation) → confirms recursive prefix stripping, not just top-key stripping.
   - Delete path → passes through unmodified, no payload.
   - Rejection/no-emit behavior when there are no new/changed leaves (mirroring the existing nil-guard fix already present in `materialize.go` for Cisco).
2. **Config validation** (`pkg/config`): unit tests for the new `DeviceProfileSonic` closed-set value and the JSON_IETF-only encoding restriction, same style as the existing `sbi_device_profile_test.go`. Cases: valid `sonic` + `JSON_IETF` accepted; `sonic` + `JSON` rejected; `sonic` + `PROTO` rejected; unaffected profiles/encodings unchanged.
3. **Dispatch** (`materialize.BuildPlan`): routing tests confirming `sbi.Type=="gnmi" && sbi.IsSonic()` calls the new encoder and that non-sonic profiles are unaffected, same style as the existing `materialize_test.go` routing tests for Cisco IOS-XR.

Only test external behavior (the produced `GnmiSetPlan`/config validation result), not internal helper functions, per existing repo test style.

No live-lab verification is required for this PR (unit tests are the bar, matching Cisco's #442 PR); a follow-up in the separate `integration-tests` repo is expected later but is out of scope here.

## Out of Scope

- Any change to SONiC/translib (`request_binder.go`, `transl_utils.go`, or any `sonic-buildimage` code) — explicitly disallowed for this workstream.
- Per-YANG-module hardcoded logic — the encoder must be schema-driven and work for any `sonic_yang` module without per-module code.
- Get/Subscribe schema-driven per-module path auto-expansion — documented as a manual `SyncConfig.Paths` workaround only; no code this iteration.
- `target.New` changes, or any other type-check sprinkled into generic infrastructure (`gnmi.go`, deviation manager, etc.) — the only legitimate dispatch point is inside `materialize.BuildPlan`, per the repo's tight-coupling rule.
- REPLACE-specific handling — this profile only ever emits gNMI `Update`, never `Replace`, so translib's REPLACE sibling-deletion behavior is never exercised and needs no special-casing.
- The docs PR (`iptecharch/docs`, new `docs/user-guide/configuration/target/device-profiles.md` page) — tracked separately, to be written after this code lands so it reflects verified behavior, not the plan.
- Live lab proof-of-concept against `clab-sonic01-sonic` — future integration-tests work, not required for this PR's completion.

## Further Notes

- This PR should be branched off the local `ciscoiosxrd2` branch and opened with PR base `ciscoiosxrd2` against `sdcio/data-server` (stacking on [#442](https://github.com/sdcio/data-server/pull/442), not `main`), pushed to `origin` (`git@github.com:sdcio/data-server.git`).
- The `sdc-protos` enum addition (`DEVICE_PROFILE_SONIC = 2`) should land on the existing open `deviceprofile` branch (base `main`) that already carries `DEVICE_PROFILE_CISCO_IOS_XR = 1` ([sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)); this data-server branch's `go.mod` should then bump to a commit on that branch that includes the new enum value.
- Sequencing dependency: the sdc-protos enum change should land/be available before or alongside the data-server encoder work, since the encoder PR wires up the gRPC/Target-CR mapping for the new profile value.
- Prior handoff context and verified live-lab facts about translib's Set/Get behavior live in a separate project workspace (`/home/mava/projects/sonic`, see `builder/README.md` and `README.md` there) — not duplicated here, but referenced as the primary source for the "verified against the live lab" claims above (specifically: the leaf-path and full-row-wrap Set examples, and the origin/`sonic_yang` vs `sonic-db` distinction).
