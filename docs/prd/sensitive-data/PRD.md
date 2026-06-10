# PRD: Sensitive Data Support

## Problem Statement

Device configuration managed by data-server may contain sensitive values — passwords, authentication keys, community strings, bearer tokens — that must reach the southbound target unchanged but must never be returned in plaintext over the northbound API. Today every northbound response (`GetIntent`, `BlameConfig`) returns leaf values verbatim, with no mechanism to classify or redact sensitive fields. Operators cannot safely expose northbound reads to dashboards, audit tools, or less-privileged users without leaking secrets.

## Solution

Introduce a **sensitive-path marker** mechanism that:

1. Lets the caller of `TransactionSet` declare which key-pruned schema paths in an intent contain sensitive values.
2. Stores those markers alongside the intent blob in `sdcio/cache`.
3. At northbound render time, unions all sensitive path sets across all intents and replaces matching leaf values with the fixed sentinel `***`.
4. Leaves the southbound path (tree merge, YANG validation, Target apply) entirely unchanged — plain values flow to the device as before.

Sensitivity is a **property of the schema path**, not a list instance. List keys are stripped from markers (e.g. `/bgp/neighbors/auth-password`, not `/bgp/neighbors[peer=10.0.0.1]/auth-password`). This eliminates staleness when list keys change and treats all instances of a leaf type uniformly.

The feature also defines an **admin bypass** — a per-request flag that privileged callers may set to receive plaintext values. The bypass is a single hook in the render pipeline, designed to be layered with stronger auth mechanisms (mTLS cert role) later without changing the redaction logic.

## User Stories

1. As a network operator, I want to mark specific leaf paths as sensitive when submitting an intent, so that northbound consumers never see plaintext passwords or secrets.
2. As a network operator, I want sensitivity to be a property of the schema path rather than a specific list instance, so that I do not have to re-declare sensitivity when list keys change.
3. As a network operator, I want `BlameConfig` to return `***` as the value of a sensitive leaf (with the owner intent still visible), so that I can see who controls the field without seeing the value.
4. As a network operator, I want `GetIntent` to return `***` for sensitive leaf values, so that exported intent data is safe to store in logs or hand to less-privileged users.
5. As a network operator, I want a sensitive marker from any intent to protect a leaf even when the highest-precedence value comes from a different intent that did not mark it sensitive, so that priority re-ordering cannot accidentally expose a secret.
6. As a network operator, I want deviation values for sensitive paths to also be masked in `WatchDeviations`, so that drift notifications do not leak plaintext secrets.
7. As a platform operator, I want an admin bypass flag on `GetIntentRequest` and `BlameConfigRequest` to retrieve plaintext values for debugging, so that authorised operators can inspect actual device configuration when needed.
8. As a platform operator, I want existing intents without sensitivity metadata to be treated as non-sensitive by default, so that the feature can be deployed without any migration.

## Implementation Decisions

### Sensitivity model

Sensitivity markers are **key-pruned schema paths** stored as a `repeated string sensitive_paths` alongside the intent blob in `sdcio/cache`. The format strips list key predicates so `/bgp/neighbors[peer=10.0.0.1]/auth-password` is stored as `/bgp/neighbors/auth-password`.

At northbound render time the system applies the sensitive-path union scoped to
the in-scope intents for that operation:

- **`GetIntent(regular)`** — only the fetched intent's own `sensitive_paths` markers apply.
- **`GetIntent(running)`**, **`BlameConfig`**, **`WatchDeviations`** — the union of all
  non-running intents' markers applies, because these operations present a merged or
  device-reflected view.

The union is computed in O(1) from an in-memory reverse index maintained on the
Datastore (see ADR 0004).  The index is populated eagerly at Datastore startup and
updated incrementally on every intent write or delete.

For the in-scope set the system:

1. Looks up the sensitive-path union from the index (or the intent's own markers for a single-intent GET).
2. Strips list keys from each leaf's path during the tree walk and checks for membership in the set.
3. Returns `***` as the string value for any matching leaf, regardless of its actual type.

The southbound path (merge, validate, apply) reads the intent blob directly and is unaware of the sensitive-path slice.

### Cross-intent rule

The cross-intent rule applies to **merged views** (`BlameConfig`, `WatchDeviations`,
`GetIntent(running)`): if any intent marks a path sensitive, the path is treated as
sensitive in those views even if the highest-precedence value comes from an intent
that did not mark it sensitive.

For **`GetIntent(regular)`** the scope is narrowed to the fetched intent's own
markers.  An intent is responsible for its own sensitivity declarations; another
intent's marker does not retroactively affect how a single-intent GET presents its
data.  See ADR 0004 for the rationale and trade-offs.

### "once sensitive → always sensitive" and the replace intent

A `replace` intent wipes all prior intents. The replace intent must explicitly carry the sensitive paths it considers sensitive. Sensitive-path slices from deleted intents are removed along with their intents. The responsibility for re-declaring sensitive paths lies with the entity performing the replace.

> **Open:** whether the `replace` intent should automatically inherit prior sensitive paths (to guard against accidental exposure during a replace operation) is left as a follow-up decision.

### Running intent

The running intent (device-reported state) is subject to the same obfuscation rule. If any intent marks a path sensitive, the running value for that path is also returned as `***` in northbound responses. This prevents `BlameConfig` from leaking plaintext via the device echo (relevant for NETCONF `GetConfig` which may return passwords verbatim).

### Cache API extension

The `sdcio/cache` library requires a new field per intent record to carry the sensitive-path slice. Two options:

- **First-class field** on the cache intent record — cleaner, requires a library PR to `sdcio/cache` before data-server work can begin.
- **Opaque metadata blob** stored as a parallel key in the cache — keeps the change data-server-internal but is less ergonomic.

> **Decision required** before implementation starts.

### Canonical path format

Key-pruned schema paths. Path encoding (gNMI with module prefixes vs internal tree path) to be decided before implementation — affects cache layout and all lookup code.

### Northbound API changes

Add `include_sensitive: bool` to `GetIntentRequest` and `BlameConfigRequest`. When `true` and the caller is authorised, plaintext values are returned. This is the admin bypass (see below).

Sensitive leaf values in responses are returned as the string `***` regardless of the leaf's YANG type. The field is present in the response (not omitted) so consumers know the field exists and is redacted.

### Admin bypass

The initial bypass mechanism is `include_sensitive: bool` in the request proto. Trust model: whoever can reach the gRPC port is trusted (same as today — the gRPC channel is currently unauthenticated). The flag is explicit in the API contract and auditable in request logs.

The bypass check is a single point in the render pipeline, before any redaction is applied, so that stronger auth (mTLS client cert role) can be layered on later without touching the redaction logic.

**Future: mTLS client cert role.** When internal gRPC mTLS is implemented (currently a TODO in config-server and data-server), bypass authorisation can be tied to a client certificate `OU=sdcio-admin` issued from a dedicated sdcio internal CA. The existing `TLS*` fields in the client config structs are already present but unimplemented. The sdcio internal CA must be separate from the kube-api CA (different trust domains).

### Module overview

| Component | Change | Notes |
|---|---|---|
| `sdc-protos` | `bool sensitive = 22` on `LeafSchema`; `bool sensitive = 24` on `LeafListSchema` | Schema-server sets flag when `sdcio-ext:sensitive` extension is present on a leaf |
| `sdc-protos` | `bool include_sensitive = 4` on `GetIntentRequest`; `bool include_sensitive = 2` on `BlameConfigRequest` | Admin bypass flag |
| `sdc-protos` | `repeated string sensitive_paths = 8` on `tree_persist.Intent` | Key-pruned paths stored alongside intent blob; absence = non-sensitive (no migration) |
| `sdcio/goyang` | One-line fix in `ApplyDeviate` — copy `Exts` for `DeviationAdd`/`DeviationReplace` | Enables `sdcio-ext:sensitive` in per-device-profile overlay YANG files |
| `pkg/tree/ops/render_opts.go` | New `RenderOpts` / `XMLRenderOpts` / `XPathRenderOpts` structs | Replaces scattered `bool` params on render ops; holds `IncludeSensitive` + `SensitivePathSet` |
| `pkg/tree/ops/sensitive.go` | `IsSensitive(e)` — reads `LeafSchema.sensitive`; `KeyPrunedPath(e)` — `SdcpbPath().ToXPath(true)` | Core helpers used by all render ops and blame |
| `pkg/tree/ops/json.go`, `proto.go`, `xml.go`, `xpath.go` | Emit `"***"` / `TypedValue{StringVal:"***"}` at leaf emit point when redaction condition is met | `!opts.IncludeSensitive && (IsSensitive(e) \|\| SensitivePathSet[KeyPrunedPath(e)])` |
| `pkg/tree/api/adapter/intentresponse.go` | `IntentResponseAdapter` forwards `RenderOpts` to ops | Carries `IncludeSensitive` and `SensitivePathSet` from `GetIntent` request |
| `pkg/tree/processors/processor_blame_config.go` | `BlameConfigProcessorParams` gains `IncludeSensitive bool`; redact `Value` and `DeviationValue` | Same condition as render ops |
| `pkg/datastore/intent_rpc.go` | Union `sensitive_paths` across all loaded intents → `SensitivePathSet`; build `RenderOpts`; pass `IncludeSensitive` from request | For `GetIntent` render |
| `pkg/datastore/datastore_rpc.go` | Same union + `IncludeSensitive` forwarding for `BlameConfig` | |
| `pkg/datastore/transaction_rpc.go` | Persist `sensitive_paths` atomically with intent blob during `TransactionSet` | Must be atomic — no window where blob exists without slice |
| `pkg/server/intent.go` | Read `req.GetIncludeSensitive()`, thread into datastore call | |
| `pkg/server/datastore.go` | Same for `BlameConfig` | |
| `WatchDeviations` emission path | Mask deviation values for sensitive paths | Both intent value and running value — Issue 05 |

### Transaction lifecycle

The sensitive-path slice is written during `TransactionSet`, atomically with the intent blob. It is deleted when the intent is deleted. If the call to write the slice fails, the intent blob write must also be rolled back.

### Migration

Existing intents in cache have no sensitive-path slice. Absence of a slice is treated as non-sensitive. No migration needed. This behaviour must be explicitly documented to avoid future misinterpretation.

## Alternative Approaches Considered

### YANG extension annotation (Alt A)

Define an `sdcio-sensitive` YANG extension module and annotate vendor YANG leaves using `deviate add { sdcio-sens:mark-sensitive; }` in per-device-profile overlay files. Schema-server exposes a `sensitive: bool` flag on `LeafSchema`. data-server checks the flag at NB render time.

**goyang finding:** `deviate add` for extension statements parses without error in `sdcio/goyang v1.6.2-2` (the `Deviate` struct carries `Extensions []*Statement`) but `ApplyDeviate` in `entry.go` silently ignores extensions — they are never copied onto the target node. A one-line fix in the sdcio/goyang fork would enable this.

This approach is complementary rather than competing. It sets a **static baseline** (leaf types always sensitive by schema definition) while the path-slice approach handles **dynamic operator control** at intent-submission time. Both resolve to the same check: "is this key-pruned path in the sensitive set?"

**Options for sourcing Alt A sensitivity without modifying vendor YANG:**

| Option | Mechanism | Scope |
|---|---|---|
| Patch sdcio/goyang | Fix `ApplyDeviate` to copy extensions; use `deviate add` in overlay YANG files | Device-profile |
| Sidecar profile file | YAML file per device-profile; schema-server reads at load time, exposes `GetSensitivePaths(profile)` RPC | Device-profile |
| Leaf-name heuristic | Schema-server auto-marks leaves named `password`, `secret`, `auth-key`, etc. | Global |
| DatastoreConfig YAML | `sensitive-paths: []string` in existing datastore config; data-server reads directly | Per-datastore |
| Drop Alt A | Path-slice only; static baseline via heuristic if needed | Per-intent |

### Sensitive flag on LeafVariant (Alt B)

Add `sensitive: bool` to the `LeafVariant` proto so the flag travels with the value through the tree. Zero path-staleness risk; natural aggregation as the flag reaches the winning leaf. Rejected for v1: requires proto changes in `sdc-protos` and updates to every importer, exporter, and blame code path — large blast radius.

### Encrypt at rest (Alt C)

Encrypt sensitive leaf values before storing in the intent blob; southbound decrypts on apply. Rejected: requires KMS integration, makes southbound stateful, and significantly increases scope.

### Sentinel + external store (Alt D)

Replace sensitive values with a sentinel in the blob; store real values in an external secrets store; southbound resolves on apply. Rejected: southbound breaks if the external store is unavailable; sentinels break YANG typed-value comparisons in tree merge.

## Open Questions

| Question | Status |
|---|---|
| sdcio/cache API extension: first-class field vs opaque sidecar? | **Resolved** — first-class `repeated string sensitive_paths = 8` on `tree_persist.Intent` (in sdc-protos); cache library stores opaque bytes unchanged |
| Canonical path format: gNMI with module prefixes or internal tree path? | **Resolved** — `SdcpbPath().ToXPath(true)` (noKeys=true); format `/bgp/neighbors/auth-password`; consistent with `must`/`when` deviation path syntax |
| Replace-intent sensitive marker inheritance: auto-inherit prior slices or require explicit re-declaration? | Open — deferred follow-up |
| Should Alt A (YANG extension / sidecar profile) be implemented alongside path-slice as a static baseline layer? | **Resolved for v1** — Alt A (goyang fix + `sdcio-ext:sensitive` + schema `sensitive` flag) is implemented as the static baseline layer (Issue 02); dynamic path-slice is the per-intent layer (Issues 03–04) |
| Admin bypass: when to layer mTLS cert role on top of the proto flag? | Deferred — depends on internal gRPC mTLS rollout |

## Out of Scope

**Encryption at rest.** Intent blobs are stored as plaintext in BadgerDB. Protecting secrets at rest requires KMS integration and is a separate infrastructure concern.

**Per-instance sensitivity.** List keys are stripped from markers — all instances of a leaf type are treated uniformly. Marking only a specific list entry's leaf as sensitive is not supported.

**Fine-grained access control.** The initial admin bypass trusts any caller that can reach the gRPC port. Role-based access control (mTLS cert role, token-based auth) is a follow-up.

**Schema-level static baseline.** Alt A (YANG extension or sidecar profile) provides a schema-level complement to the path-slice approach. Implementing it is not part of this PRD but is designed to compose cleanly with it.

**Audit log.** Recording which calls exercised the admin bypass is not part of this PRD.
