# Spec: segregate `cache.Client`/`CacheClientBound` by capability

**Status:** ready-for-agent

**Design record:** [`pkg/cache/docs/adr/0002-segregate-cache-client-by-capability.md`](../../pkg/cache/docs/adr/0002-segregate-cache-client-by-capability.md) (cross-references [ADR 0001](../../pkg/cache/docs/adr/0001-config-server-backed-cache-client.md)). This spec is the execution plan; the ADR is the "why."

## Problem Statement

`cache.Client`/`CacheClientBound` (`pkg/cache/cache.go`, `pkg/cache/cacheClientBound.go`) are each one flat, 13-method interface that both `LocalCache` and `ConfigServerCache` must implement in full, even though the two backends fundamentally disagree on two of those methods:

- `ConfigServerCache.InstanceIntentModify`/`InstanceIntentDelete` are unconditional no-ops (config-server/kube-api owns real-Intent writes), but the interface gives no signal that these calls are decorative — `pkg/datastore/transaction_rpc.go`'s write path checks their errors and updates `sensitivePathIndex` as if persistence happened, regardless of backend.
- `ConfigServerCache`'s instance-lifecycle methods (`InstanceCreate`/`Delete`/`Close`/`Exists`/`InstancesList`) only ever govern its in-memory `running` store — they have no relationship to real-Intent reads under that backend, unlike `LocalCache` where lifecycle governs the one shared disk store underlying everything.
- Every consumer that only reads real Intents (`forEachIntent`, `populateSensitivePathIndex`) still depends on the full `CacheClientBound`, so their tests set up mock expectations for methods they never call.
- `LocalCache` duplicates its bytes→`ImportConfigAdapter` decode logic three times (`InstanceIntentGet`, the `InstanceIntentGetAll` channel bridge, `InstanceRunningGet`).
- Config-server's `ImportConfigAdapter` implementation (`documentImporter`, `mergeConfigBlobs`) lives inside the cache-layer package (`pkg/cache/configserver/`) instead of alongside its sibling tree importers (`pkg/tree/importer/{proto,json,xml}/`).

## Solution

Split `Client`/`CacheClientBound` into four capability interfaces — `IntentReader`, `IntentWriter`, `RunningStore`, `InstanceLifecycle` — composed back into `Client`/`CacheClientBound` for callers that need the full set. Move config-server's no-op write behavior into a generic, named `noopIntentWriter` composed at the factory (`Server.createCacheClient`) instead of living as methods on `ConfigServerCache` itself. Deduplicate `LocalCache`'s decode logic into one private helper. Relocate config-server's `ImportConfigAdapter` implementation to `pkg/tree/importer/configserver/`.

## User Stories

1. As a data-server maintainer, I want `cache.Client` split into capability interfaces, so that a backend's inability to write real Intents is visible in its type signature rather than hidden inside a silently-succeeding method body.
2. As a data-server maintainer, I want `CacheClientBound` to mirror that same split, so that `Datastore`-level helper functions (`forEachIntent`, `populateSensitivePathIndex`) can depend on only the capability they actually use.
3. As a test author, I want to mock `IntentReader` alone for read-only code paths, so that I don't have to set up expectations for `IntentWriter`/`RunningStore`/`InstanceLifecycle` methods my test never calls.
4. As a data-server maintainer, I want `InstanceLifecycle` isolated as its own interface, so that `ConfigServerCache`'s lifecycle methods (which only ever govern its in-memory running store) don't masquerade as "does config-server have data for this target."
5. As a future backend implementer, I want a generic `noopIntentWriter` available in `pkg/cache`, so that I can compose a read-only `IntentReader` backend into a full `Client` without writing my own no-op boilerplate.
6. As a data-server maintainer, I want `LocalCache`'s bytes→`ImportConfigAdapter` decode logic in one place, so that a bug in decoding (e.g. a future default-field change) is fixed once instead of at three call sites.
7. As a data-server maintainer, I want config-server's `ImportConfigAdapter` implementation (`documentImporter`, `mergeConfigBlobs`) to live under `pkg/tree/importer/configserver/`, so that every `ImportConfigAdapter` implementation is discoverable in one place instead of two.
8. As a future reader of `pkg/cache`, I want ADR 0002 to record why the interface split happened and which alternatives were rejected, so that nobody "fixes" the segregation back into one flat interface without understanding the no-op/lifecycle motivations.
9. As a data-server maintainer, I want `Cache.Type: local` to require zero behavior change from this refactor, so that the existing, already-shipped local backend isn't put at risk by a change motivated by the newer config-server backend.
10. As a code reviewer, I want `Server.createCacheClient` to remain the only place that composes backend-specific behavior (e.g. instantiating `noopIntentWriter` for config-server), so that the repo's no-tight-coupling rule holds and generic datastore code never branches on `Cache.Type`.

## Implementation Decisions

- `cache.Client`/`CacheClientBound` (`pkg/cache/cache.go`, `pkg/cache/cacheClientBound.go`) split into `IntentReader`, `IntentWriter`, `RunningStore`, `InstanceLifecycle`; `Client`/`CacheClientBound` remain as exported names, now composed by embedding all four.
- `ConfigServerCache` implements `IntentReader` + `RunningStore` + `InstanceLifecycle` directly; its existing `InstanceIntentModify`/`InstanceIntentDelete` no-op methods are removed from the type.
- New generic `noopIntentWriter` in `pkg/cache` (unexported type, not scoped to the `configserver` sub-package) implements `IntentWriter` as unconditional no-ops. It is composed into the full `Client` only at `Server.createCacheClient`'s `config-server` case — the one legitimate type-switch point per the repo's no-tight-coupling rule.
- `forEachIntent` and `populateSensitivePathIndex` (`pkg/datastore/transaction_rpc.go`, `pkg/datastore/datastore_rpc.go`) narrow their parameter type from `cache.CacheClientBound` to `cache.IntentReader`. `Datastore`'s own `cacheClient` field stays a single `cache.CacheClientBound` — no struct changes needed; Go's structural typing lets the narrower parameter types accept it as-is.
- `LocalCache` (`pkg/cache/local.go`) gains a private `decodeIntent(b []byte) (importer.ImportConfigAdapter, error)` method, called from `InstanceIntentGet`, the `InstanceIntentGetAll` channel bridge, and `InstanceRunningGet` — replacing three duplicated inline decode blocks (`proto.Unmarshal` into `tree_persist.Intent`, then `treeproto.NewProtoTreeImporter`).
- `documentImporter` and `mergeConfigBlobs` (and their tests) move unchanged from `pkg/cache/configserver/{importer.go,merge.go,importer_test.go,merge_test.go}` to a new `pkg/tree/importer/configserver/` package. `ConfigServerCache.InstanceIntentGet`/`InstanceIntentGetAll` (`pkg/cache/configserver.go`) import the new package instead of the old location.
- `pkg/cache/configserver/` retains only the reader seam: `localread.go`, `grpc.go`, `fake.go` (+ their tests).
- No standalone `DocumentTreeImporter` type is introduced — `documentImporter` continues embedding `JsonTreeImporter` and overriding `GetOrphan`/`GetSensitivePaths`, just relocated.
- ADR 0001 is not amended; `pkg/cache/docs/adr/0002-segregate-cache-client-by-capability.md` (already written) records this decision.
- `mocks/mockcacheclient/*` regenerate against the new interface shapes — mechanical fallout, not a further design decision.

## Testing Decisions

- Tests exercise the segregated interfaces through their external behavior (what a caller observes), not the internal composition wiring or `decodeIntent`'s existence as a symbol.
- `pkg/cache/local_test.go`: confirm `InstanceIntentGet`, `InstanceIntentGetAll`, and `InstanceRunningGet` all still round-trip a `*tree_persist.Intent` correctly after the `decodeIntent` extraction — a regression check, not new behavior.
- `pkg/cache/configserver_test.go`: add compile-time assertions that `*ConfigServerCache` satisfies `IntentReader`, `RunningStore`, and `InstanceLifecycle`, and does **not** need an `IntentWriter` test at all — there is no method left on the type to test for no-op behavior.
- New tests for `noopIntentWriter`: minimal — confirm both methods return `nil` unconditionally, regardless of input.
- `pkg/server/cache_test.go`: verify `createCacheClient`'s `config-server` case composes a `Client` whose `IntentWriter` behavior is the generic no-op — i.e., calling `IntentModify`/`IntentDelete` on the composed client returns `nil` without reaching the reader seam.
- `pkg/datastore` tests (`transaction_rpc_test.go`, `sync_test.go`, `sensitive_path_union_test.go`, `datastore_test.go`): update mock expectations for the new interface shapes. Tests exercising `forEachIntent`/`populateSensitivePathIndex` should construct a narrower `IntentReader` mock instead of a full `CacheClientBound` mock — treat a shrinking mock-setup diff here as the concrete signal the split is paying off.
- Prior art: `pkg/cache/local_test.go`, `pkg/cache/configserver_test.go`, and `pkg/cache/configserver/fake_test.go` establish the existing patterns (table-driven cases, `FakeLocalConfigReader` for config-server scenarios) to follow.
- Final check: `go build ./...`, `go vet ./...`, and the full `go test ./...` suite pass, confirming every consumer compiles against the new interface shapes.

## Out of Scope

- Amending ADR 0001 — it remains an unmodified record of already-shipped reasoning; superseding pieces are recorded via ADR 0002 instead.
- Extracting a standalone `DocumentTreeImporter` type — deferred until a second config-server-shaped document source exists (see ADR 0002's considered options).
- Any change to config-server's real write path, RPC contract, or field-mapping table (ADR 0001's scope) — this effort only touches data-server's local interface shape.
- Fixing the permanently-empty `GetDeletes()`/`ExplicitDeletes` gap on the config-server-backed importer — a documented, pre-existing limitation (ADR 0001), unrelated to this refactor.
- Any behavior change to `Cache.Type: local` — this is a pure interface segregation with zero on-disk or runtime behavior change for the existing backend.

## Further Notes

- The interface split (stories 1–6, 9–10) and the package relocation (story 7) are independent of each other — if the implementing agent finds the mock/test fallout from the interface split larger than expected in one session, splitting into two PRs is reasonable. The relocation doesn't depend on the split, or vice versa.
- This spec was synthesized from a `/grilling` session (not re-run here); ADR 0002 already captures the rejected alternatives and reasoning behind each decision above, so consult it directly rather than re-deriving "why" from this spec's "what."
