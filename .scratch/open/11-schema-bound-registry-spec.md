# Spec: Shared, refcounted schema-bound registry

**Status:** ready-for-agent  
**Tracker:** local (`.scratch/open/`)  
**Related:** ADR [`docs/adr/0005-shared-refcounted-schema-bound-registry.md`](../../docs/adr/0005-shared-refcounted-schema-bound-registry.md)  
**Anchor branch:** `schema-bound-registry` (based on `main`, independent of PR [#508](https://github.com/sdcio/data-server/pull/508) / PR [#511](https://github.com/sdcio/data-server/pull/511))

---

## Problem Statement

Every datastore builds its own private `SchemaClientBoundImpl`, each holding its own `sync.Map` cache of schema lookups keyed by path. Datastores that share the same YANG schema identity (name, vendor, version) — e.g. multiple targets running the same NOS/version — duplicate the same cached schema entries once per datastore, with no upper bound on how many datastores can exist. Separately, `remoteClient` (the schema-server gRPC client) carries its own `ttlcache`-based cache and `RemoteSchemaCache` config knob, which is now redundant once caching is deduplicated at the schema-bound layer, and only exercises time-based expiry that isn't needed for schema data that changes on reload, not on a clock.

## Solution

Introduce a **schema client registry**: a small, process-wide pool of `SchemaClientBoundImpl` instances keyed by schema identity (name, vendor, version), refcounted per datastore. A datastore acquires a **schema-bound handle** from the registry when it's created and calls `Close()` on that handle when it's deleted; the registry evicts the pooled instance once no datastore holds a handle to it. The pooled instance's internal structure (path-keyed lookup cache, root-ambiguity cache, per-entry coalescing) is unchanged — only its construction and lifecycle move from "one per datastore" to "one per schema identity, refcounted." Remove `remoteClient`'s `ttlcache` and the `RemoteSchemaCache` config it was driven by, so the registry becomes the single schema-caching mechanism in the process; this includes the northbound `GetSchema`/`GetSchemaDetails` RPC passthrough, which loses caching as an accepted simplification. No TTL is introduced anywhere.

---

## User Stories

1. As an operator running many datastores against the same NOS/version, I want schema lookups cached once per schema identity instead of once per datastore, so that memory use doesn't scale with the number of datastores.
2. As an operator running datastores against different NOS/versions, I want each schema identity's cache to be fully independent, so that one vendor's schema entries never leak into another's lookups.
3. As a maintainer, I want deleting a datastore to release its hold on the shared schema cache, so that a schema identity no longer used by any datastore is eventually freed instead of leaking for the life of the process.
4. As a maintainer, I want the pooled instance's construction to stay exactly as it is today (same `sync.Map`, same per-entry coalescing, same root-ambiguity field), so that pooling adds no new correctness surface to get wrong.
5. As a maintainer, I want `Datastore` to acquire and release its schema-bound handle without needing to remember the registry exists afterward, so that no call site can forget to release and leak a refcount.
6. As a maintainer, I want `remoteClient`'s `ttlcache` and `RemoteSchemaCache` config removed once the registry covers the caching need, so that there is exactly one schema-caching mechanism in the codebase, not two.
7. As a developer reading test fakes for `SchemaClientBound`, I want the shared interface unchanged (no `Close()` added to it), so that every existing read-only consumer (tree context, converter, XML/NETCONF encoders, test fakes) is unaffected by this change.
8. As a developer, I want `Datastore.Stop()` (process shutdown) to NOT release the registry handle, so that shutdown and explicit datastore deletion remain the two distinct, non-overlapping teardown paths they are today.
9. As a developer, I want the northbound `GetSchema`/`GetSchemaDetails`/etc. RPC passthrough to keep working exactly as it does today (minus caching), so that no behavior beyond the removed cache changes for that call path.
10. As a developer, I want a defensive no-op when `Release` is called on a schema identity that's already at zero refcount or was never registered, so that a bug in call ordering doesn't panic the process.

---

## Implementation Decisions

### Schema client registry

- New type in `pkg/datastore/clients/schema/` (same package as `SchemaClientBoundImpl`), holding `map[tripleKey]*pooledEntry` behind a mutex, where `tripleKey` is a comparable struct of `{Name, Vendor, Version string}` and `pooledEntry` wraps `*SchemaClientBoundImpl` plus a refcount.
- `GetOrCreate(cfg *config.SchemaConfig, sc schema.Client) SchemaClientBound` — looks up by triple; on hit, increments refcount and returns a handle wrapping the existing pooled instance; on miss, constructs via the existing `NewSchemaClientBound(cfg, sc)` unchanged, stores it with refcount 1, and returns a handle.
- The handle is a small wrapper implementing `SchemaClientBound` by delegating both methods to the pooled instance, plus a `Close()` method (not part of the `SchemaClientBound` interface) that calls back into the registry to decrement the refcount and evict the pooled entry when it reaches zero.
- `Release` on an unknown or already-zero triple is a no-op (defensive, not an error).
- `SchemaClientBound` interface itself is not modified — no `Close()` added there. `NewSchemaClientBound` remains available unchanged for the ~20 existing test call sites that construct a bound instance directly, bypassing the registry entirely.

### Datastore wiring

- The registry is constructed once in `pkg/server/schema.go`, alongside `schema.Client` construction (local or remote), and threaded into `pkg/server/datastore.go`'s two `datastore.New(...)` call sites (explicit `CreateDataStore` RPC and startup config loading) in place of the raw `schema.Client` currently passed for this purpose.
- `Datastore.New` acquires its schema-bound handle via `registry.GetOrCreate(...)` internally (replacing its current unconditional call to `NewSchemaClientBound`), and stores it on the `Datastore` struct with the handle's concrete type (not the bare interface), so `Close()` is directly callable without a type assertion.
- `Datastore.Delete()` calls `d.schemaClient.Close()` as part of teardown.
- `Datastore.Stop()` does not call `Close()` — shutdown and explicit deletion remain mutually exclusive teardown paths, as they are today.

### Removing `remoteClient`'s cache

- Remove the `ttlcache` field, its construction, and all `Get`/`Set`/`Range`/`Delete` cache logic from `pkg/schema/remote.go` (`GetSchema`, `DeleteSchema`, `ToPath`, `GetSchemaElements` all currently branch on `c.schemaCache == nil`; all branches collapse to the uncached path).
- Remove `RemoteSchemaCache` from `pkg/config/config.go` (the `Cache *RemoteSchemaCache` field on `RemoteSchemaServer`, the `RemoteSchemaCache` struct itself, and its `validateSetDefaults` defaulting logic).
- `NewRemoteClient`'s signature drops the `cacheConfig *config.RemoteSchemaCache` parameter.

---

## Testing Decisions

- **Principle:** test observable behavior at the registry's boundary (its exported `GetOrCreate`/handle `Close()` surface), not the unchanged internals of `SchemaClientBoundImpl`.
- **Primary seam:** the schema client registry itself, in isolation:
  - Two `GetOrCreate` calls with the same triple return handles backed by the same pooled `*SchemaClientBoundImpl` (assert via a distinguishing side effect, e.g. populating the path cache through one handle and observing the hit through the other).
  - `GetOrCreate` calls with different triples return handles backed by distinct pooled instances.
  - Closing one of two handles sharing a triple keeps the pooled instance alive (refcount > 0); closing the last handle evicts it (a subsequent `GetOrCreate` for that triple constructs fresh).
  - `Close()` called twice on the same handle, or `Release` on an unregistered triple, does not panic.
- **Datastore wiring:** thin, so no new dedicated seam — existing `pkg/server` datastore lifecycle tests (create/delete) should continue passing unchanged, proving the wiring didn't break datastore construction/teardown; no new assertions about the registry needed at that layer.
- **`remoteClient` cache removal:** existing `pkg/schema` tests for `remoteClient` should continue passing with the cache branches deleted (dead-code removal, not new behavior); no new tests required beyond confirming `NewRemoteClient` compiles without the removed parameter.
- **Prior art:** `pkg/datastore/clients/schema/schemaClientBound.go`'s existing `sync.Map` + per-entry-mutex coalescing pattern (`SchemaIndexEntry`) is the reference for "don't touch this," not something to re-test.

---

## Out of Scope

- Invalidating individual schema paths on `ReloadSchema`/`DeleteSchema` (generation/versioning of cached entries) — deferred, tracked as a follow-up in ADR 0005.
- Evicting individual schema paths when a config branch is deleted — deferred; this spec only evicts whole pooled instances when a schema identity has no remaining datastore.
- Weak-reference-based automatic reclamation (`weak.Pointer` with strong refs from tree nodes) — considered and deferred in ADR 0005; refcounting on datastore create/delete is the chosen mechanism for this spec.
- Any change to `RootAmbiguityRegistry`'s behavior or `SchemaClientBoundImpl`'s internals — it is pooled as-is; whatever fields PR #511 later adds to it (e.g. `rootAmbiguity`) are pooled automatically without further changes here.
- Refcounting or pooling at any granularity other than the full schema identity triple.

---

## Further Notes

- This branch (`schema-bound-registry`) is intentionally based on `main`, not stacked on PR #508 or #511, so it can merge independently. See ADR 0005's "Considered options" for why pooling the whole instance (rather than a flat path-keyed map) makes this safe: PR #511 only adds fields to `SchemaClientBoundImpl`, it doesn't restructure the methods this spec touches.
- Domain glossary: see repo root `CONTEXT.md` ("Schema caching" section — schema client registry, schema-bound handle) on this branch.
- Suggested skills for the next session: `/tdd` for the registry's red-green cycle, then `/code-review` before committing, per the standard `/implement` flow.
