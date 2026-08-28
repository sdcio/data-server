# 02 — Make config-server's write no-op explicit via `noopIntentWriter`

**What to build:** `ConfigServerCache`'s `InstanceIntentModify`/`InstanceIntentDelete` no-op methods are removed from the type entirely (`pkg/cache/configserver.go`), so `ConfigServerCache` now implements only `IntentReader` + `RunningStore` + `InstanceLifecycle`, not `IntentWriter`. A new generic, unexported `noopIntentWriter` type is added directly in `pkg/cache` (not scoped to the `configserver` sub-package) implementing `IntentWriter` as unconditional no-ops. `Server.createCacheClient`'s `config-server` case (`pkg/server/cache.go`) is the only place that composes `noopIntentWriter` together with `*ConfigServerCache` into a full `Client` — this is the one legitimate type-switch point per the repo's no-tight-coupling rule; no other generic code branches on `Cache.Type`. After this ticket, a caller holding `s.cacheClient` as `cache.Client` under `Cache.Type: config-server` gets real reads/lifecycle from `*ConfigServerCache` and a visibly-named, always-nil write path from `noopIntentWriter`, instead of the no-op being buried inside `ConfigServerCache`'s own method bodies.

**Blocked by:** 01 — needs the `IntentReader`/`IntentWriter`/`RunningStore`/`InstanceLifecycle` interfaces to exist.

**Status:** done

- [x] `ConfigServerCache.InstanceIntentModify`/`InstanceIntentDelete` methods are deleted from `pkg/cache/configserver.go`.
- [x] New unexported `noopIntentWriter` type in `pkg/cache` implements `IntentWriter`; both methods return `nil` unconditionally regardless of input.
- [x] New tests for `noopIntentWriter` confirm both methods return `nil` unconditionally (minimal, per spec).
- [x] `pkg/cache/configserver_test.go` adds compile-time assertions that `*ConfigServerCache` satisfies `IntentReader`, `RunningStore`, and `InstanceLifecycle` — and does **not** attempt to satisfy `IntentWriter` on its own (no test asserts no-op behavior against the bare type, since there's no method left on it to test).
- [x] `Server.createCacheClient`'s `config-server` case (`pkg/server/cache.go`) composes `*ConfigServerCache` with `noopIntentWriter` into the full `Client` it assigns to `s.cacheClient`.
- [x] `pkg/server/cache_test.go` verifies `createCacheClient`'s `config-server` case produces a `Client` whose `IntentModify`/`IntentDelete` return `nil` without reaching the reader seam (e.g. without calling into the underlying `configserver.LocalConfigReader`).
- [x] `Cache.Type: local`'s behavior is unaffected — `LocalCache` keeps its own real `InstanceIntentModify`/`InstanceIntentDelete` methods untouched.
- [x] `go build ./...`, `go vet ./...`, and `go test ./...` all pass.

## Comments

- Landed via TDD across three red→green cycles: (1) `noopIntentWriter` added standalone in `pkg/cache/noop_intent_writer.go`, tested for unconditional-nil behavior; (2) `ConfigServerCache.InstanceIntentModify`/`InstanceIntentDelete` deleted from `pkg/cache/configserver.go`, breaking its old `Client`/no-op tests (red), replaced with `IntentReader`/`RunningStore`/`InstanceLifecycle`-only compile assertions in `configserver_test.go` (green); (3) new exported `cache.NewConfigServerClient` composes `*ConfigServerCache` + `noopIntentWriter` into a full `Client` via struct embedding, wired into `Server.createConfigServerCacheClient` (`pkg/server/cache.go`), with `pkg/server/cache_test.go` asserting the composed client's writes are no-ops. Full `go build`/`go vet`/`go test ./...` pass; `Cache.Type: local` untouched.
