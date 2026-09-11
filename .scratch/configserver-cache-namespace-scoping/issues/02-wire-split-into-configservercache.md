# 02 — `ConfigServerCache` derives namespace/name per call, drops fixed `namespace` field

**What to build:** `ConfigServerCache` stops trusting a single, deployment-wide `namespace` field and instead derives both the target namespace and bare target name from each call's `cacheInstanceName`, using the split helper from ticket 01. One data-server process can now correctly serve datastores whose Target CRs live in different Kubernetes namespaces, and config-server's `ConfigReadService` receives the bare target name (not the whole compound datastore name) as `TargetName`. A malformed datastore name fails loudly and specifically — as `ErrMalformedDatastoreName` — at the point it's decoded, both on read and at datastore-creation time, rather than surfacing later as a generic downstream error or silently building a lookup with an empty namespace/name.

Concretely:
- `ConfigServerCache.target(cacheInstanceName string)` changes signature from returning `configserver.Target` to `(configserver.Target, error)`, calling the split helper.
- `ConfigServerCache.namespace` struct field is deleted. `NewConfigServerCache`/`NewConfigServerClient` drop their `namespace` parameter and become single-argument constructors (just the `configserver.LocalConfigReader`). No fallback/default namespace is retained anywhere.
- Every caller of `target()` — `InstanceIntentsList`, `InstanceIntentGet`, `InstanceIntentExists`, `InstanceIntentGetAll` — propagates the new error. `InstanceIntentGetAll` sends it on its existing `errChan`, the same channel real `reader.List`/`reader.Get` errors already use.
- `InstanceCreate` also calls the split helper (discarding the successful result — it only needs the error) so a malformed datastore name fails at datastore-creation time, not silently, only to surface later on the first real-Intent read.
- The one production call site, `pkg/server/cache.go`'s `createConfigServerCacheClient`, is updated to call `cache.NewConfigServerClient(reader)` without a namespace argument (it may still log using `s.config.Cache.Namespace` for now — ticket 03 removes that field).

**Blocked by:** 01

**Status:** done

- [x] `target()` returns `(configserver.Target, error)`, built from the ticket-01 split helper
- [x] `ConfigServerCache.namespace` field removed; `NewConfigServerCache`/`NewConfigServerClient` take only a `configserver.LocalConfigReader`
- [x] `InstanceIntentsList`/`InstanceIntentGet`/`InstanceIntentExists` return the split error as a normal error when `target()` fails
- [x] `InstanceIntentGetAll` sends the split error on `errChan` (closing both channels), exercised through the existing channel-draining test pattern
- [x] `InstanceCreate` rejects a malformed datastore name before ever touching the `running` map (instance is not left half-created)
- [x] `pkg/server/cache.go`'s config-server call site compiles against the new single-argument constructor
- [x] `pkg/cache/configserver_test.go`: cases exercising `target()`'s new error return directly (`errors.Is(err, ErrMalformedDatastoreName)`), confirming `InstanceIntentsList`/`InstanceIntentGet`/`InstanceIntentExists` surface it as a normal returned error, a case for `InstanceIntentGetAll`'s `errChan` path, and a case for `InstanceCreate` rejecting before mutating `running`
- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass

## Comments

- Landed in `pkg/cache/configserver.go` (+ updated `pkg/cache/configserver_test.go`, `pkg/server/cache.go`): `target()` now derives `configserver.Target` per call via `splitDatastoreName`, constructors dropped their `namespace` parameter, and all four `target()` callers plus `InstanceCreate` propagate/surface `ErrMalformedDatastoreName`.
