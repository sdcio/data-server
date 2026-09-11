# Spec: derive target namespace/name from the datastore name in `ConfigServerCache`

**Status:** ready-for-agent

This spec was synthesized from a `/grilling` session (not re-run here).

## Problem Statement

`ConfigServerCache` (`pkg/cache/configserver.go`) scopes every real-Intent read against a single, fixed `namespace` value, taken from a deployment-wide `CacheConfig.Namespace` config field. This is wrong on two counts, both on the same line (`target()`, `pkg/cache/configserver.go:77-79`):

- SDC is multi-namespace: a single data-server process serves datastores whose underlying Target CRs live in different Kubernetes namespaces. A fixed, deployment-wide namespace can only ever be correct for one of them.
- Even if the namespace were fixed correctly, `target()` passes the entire datastore name (e.g. `"prod.srl1"`) as `configserver.Target.Name` — the bare target name config-server's `ConfigReadService` actually expects (`TargetName`) — producing a malformed lookup regardless of the namespace bug.

config-server itself never sends namespace as a separate field: it names every datastore it creates on data-server as `<target namespace>.<target name>` (`storebackend.Key.String()`), and that compound string is the only place namespace information travels. Data-server currently has no code path that decodes it.

## Solution

Delete the fixed `CacheConfig.Namespace` config field entirely. Instead, `ConfigServerCache` derives both the target namespace and the bare target name by splitting the datastore name (received as `cacheInstanceName` on every `Client` call) on its first `.` character, exactly mirroring config-server's own encoding. This is a config-server-backend-only concern, so the split lives entirely inside `ConfigServerCache` — no generic `Datastore`/`server` code needs to know a datastore name is ever compound.

## User Stories

1. As a data-server maintainer, I want `ConfigServerCache` to derive each call's target namespace from the datastore name it's already given, so that one data-server process correctly serves datastores across multiple Kubernetes namespaces instead of being pinned to one.
2. As a data-server maintainer, I want the bare target name (not the whole compound datastore name) sent to `ConfigReadService` as `TargetName`, so that config-server's lookup isn't malformed regardless of the namespace fix.
3. As a data-server maintainer, I want the namespace/name split confined to `ConfigServerCache`, so that generic `Datastore`/`server` code keeps treating the datastore name as one opaque string, per this repo's no-tight-coupling rule.
4. As a data-server operator, I want `Cache.Type: config-server` to require no `namespace` setting in the deployment config at all, so that I can't misconfigure a value that was never meaningful once namespace became per-datastore.
5. As a data-server maintainer, I want a malformed datastore name (one with no namespace component) to fail loudly and specifically at the point it's decoded, so that a real invariant violation surfaces as a data-server-authored error instead of a generic downstream `InvalidArgument` from config-server's `ConfigReadService`.
6. As a future reader of `pkg/cache`, I want `CONTEXT.md` to define "datastore name," "target namespace," and "target name" precisely, so that "namespace" in this codebase always means the same thing and isn't confused with the compound datastore-name string.
7. As a test author, I want the namespace/name split logic unit-testable in isolation from the rest of `ConfigServerCache`, so that dotted/undotted/multi-dot/empty-string cases can be covered without standing up the full cache client.

## Implementation Decisions

- `CacheConfig.Namespace` (`pkg/config/datastore.go`) is deleted, along with its yaml/json tags and the `c.Namespace == ""` requirement in `CacheConfig.validateSetDefaults()`'s `config-server` case.
- `ConfigServerCache.namespace` (struct field) is deleted. `NewConfigServerCache`/`NewConfigServerClient` drop their `namespace` parameter — both become single-argument constructors taking only the `configserver.LocalConfigReader`. No fallback/default namespace is retained anywhere.
- A new standalone helper splits a datastore name into its namespace/name components: split on the **first** `.` only (never a naive multi-part split), because a Kubernetes namespace is always a DNS-1123 label (no dots), while a target's bare name may itself legally contain dots. A datastore name is malformed — and the helper returns an error — whenever there's no dot at all, **or** when either resulting segment (namespace or name) is empty (e.g. `".srl1"`, `"prod."`): the helper's contract is "return a `configserver.Target` with both fields guaranteed non-empty, or fail," not merely "check a dot exists."
- `ConfigServerCache.target(cacheInstanceName string)` changes signature from returning `configserver.Target` to returning `(configserver.Target, error)`, calling the new split helper and surfacing a data-server-authored error when the name is malformed by the above definition, rather than silently building a `Target` with an empty namespace or name.
- The malformed-name error is a package-level sentinel (e.g. `ErrMalformedDatastoreName`), matching this same package's existing `ErrNotFound`/`ErrRunningNotFound` convention, so callers can `errors.Is`-check it if needed. No gRPC status-code mapping (e.g. `codes.InvalidArgument`) is introduced for it as part of this spec — that's a broader, unrelated error-handling policy question (whether *any* cache-layer error gets special gRPC-code treatment today) that this fix shouldn't be the one to decide; it surfaces as a normal error like any other cache-layer failure.
- Every caller of `target()` — `InstanceIntentsList`, `InstanceIntentGet`, `InstanceIntentExists`, `InstanceIntentGetAll` — propagates that new error. `InstanceIntentGetAll` (which streams via channels rather than returning `(x, error)`) sends it on its existing `errChan`, the same channel real `reader.List`/`reader.Get` errors already use — no second error-signaling path is introduced.
- `InstanceCreate` also calls the split helper (discarding the successful result — it only needs the error) so a malformed datastore name fails at datastore-creation time, not silently, only to surface later on the first real-Intent read. Today `InstanceCreate`/`InstanceDelete`/`InstanceExists`/`InstancesList`/`InstanceClose` only touch the in-memory `running` map and never call `target()`; this adds the one validating call to `InstanceCreate` without otherwise changing those methods' behavior.
- The dotted encoding is treated as mandatory for `Cache.Type: config-server`: no code path silently tolerates or auto-repairs a dotless (or partially-empty) datastore name for this backend.
- **Considered and rejected**: an explicit namespace field (on `CreateDataStoreRequest` or elsewhere in `sdc-protos`) instead of parsing. Rejected for now — `ConfigServerCache` is a single process-wide singleton distinguishing datastores only by the opaque `cacheInstanceName string` every `Client`/`CacheClientBound` method already takes, so avoiding the parse would require threading a structured identifier (e.g. `DatastoreID{Namespace, Name}`) through the entire `Client`/`CacheClientBound` interface (all methods, both backends, per ADR 0002) plus a coordinated cross-repo proto/rollout change — a substantially larger effort than this fix, for a backend-specific concern `LocalCache` has no use for. No ADR was written recording this rejection; revisit if config-server's dotted-name convention ever needs to change.
- `pkg/cache/CONTEXT.md` gains three glossary entries (already added during the grilling session): **Datastore name** (the existing, unchanged meaning — the opaque end-to-end identifier, compound only under the config-server backend), and **Target namespace** / **Target name** (the two split-out components, reusing config-server's own vocabulary — `TargetNamespaceKey`/`TargetNameKey`, `GetConfigRequest.TargetNamespace`/`TargetName` — rather than inventing new terms).

## Testing Decisions

- The namespace/name split is implemented as its own standalone, pure function (not inlined into `target()`), specifically so it can be table-driven unit-tested in isolation: dotted (`"prod.srl1"` → `"prod"`/`"srl1"`), dotted-with-extra-dots-in-the-name (`"prod.rack1.srl1"` → `"prod"`/`"rack1.srl1"`), no-dot (error), empty-namespace (`".srl1"`, error), and empty-name (`"prod."`, error) cases.
- `pkg/cache/configserver_test.go`: add cases exercising `target()`'s new error return directly (asserting `errors.Is(err, ErrMalformedDatastoreName)`), and confirm `InstanceIntentsList`/`InstanceIntentGet`/`InstanceIntentExists` surface it as a normal returned error.
- `pkg/cache/configserver_test.go`: add a case confirming `InstanceCreate` rejects a malformed datastore name before ever touching the `running` map (i.e. the instance is not left half-created).
- `pkg/cache/configserver_test.go` (or a new `_test.go` alongside `InstanceIntentGetAll`): confirm the split error arrives on `errChan` exactly like a real `reader.List` error would, exercised through the existing channel-draining test pattern.
- `pkg/config/datastore_test.go`: remove the three `Namespace: "sdcio"`-bearing cases (`"config-server type requires a namespace"`, and the two others that set `Namespace: "sdcio"` incidentally); confirm `config-server` type validation still requires `Address` but no longer mentions `Namespace` at all.
- `pkg/server/cache_test.go`: update `TestCreateConfigServerCacheClient`/`TestCreateConfigServerCacheClient_WritesAreNoOps` to drop `Namespace: "sdcio"` from the `CacheConfig` literal (construction should succeed without it).
- Tests exercise external behavior (what `target()`/its callers return or send on `errChan`), not the split helper's existence as a symbol beyond its own direct table-driven test.
- Final check: `go build ./...`, `go vet ./...`, and the full `go test ./...` suite pass.

## Out of Scope

- The config-server-backed intent-diff bug found during the same investigation (config-server's `ConfigReadService` serves live `Spec` rather than a distinct "last-applied" value, defeating data-server's old-vs-new diff mechanism for updates and deletions) — written up separately as its own handoff (`/tmp/data-server-configserver-diff-bug-handoff.md`) for a dedicated `/grilling` session, since its fix shape isn't decided and may have cross-repo (sdc-protos/config-server) implications.
- Any change to config-server's `CreateDataStoreRequest`/`sdcpb.Target` proto shape, or to how config-server itself names datastores (`storebackend.Key.String()`) — this spec only changes how data-server *decodes* the existing encoding, not the encoding itself.
- Any behavior change to `Cache.Type: local` or `Cache.Type: remote` — this is scoped entirely to the `config-server` cache backend.
- Adding a data-server-side notion of "default namespace" for datastores created outside config-server's Target controller (e.g. manually, via direct API calls, or in docs examples) — per the grilling session, the dotted form is mandatory for this backend, and a dotless name is a hard error, not a fallback case.

## Further Notes

- The two bugs described in the Problem Statement (fixed namespace; whole datastore name sent as bare target name) are fixed together by the same split, since both are corrected by `target()` deriving both components correctly — there's no meaningful way to land one without the other.
- The parse-vs-explicit-field trade-off (see "Considered and rejected" above) was deliberately re-examined mid-grilling and the parsing approach was reaffirmed as-is, with no ADR written. If config-server's `<namespace>.<name>` dotted convention (`storebackend.Key.String()`) ever changes, or if a second consumer starts relying on this same parsing, that's the trigger to revisit this decision — and likely the point an ADR becomes warranted, since by then it'll be a decision with real history to record.
