# 01 — Split `cache.Client`/`CacheClientBound` into capability interfaces

**What to build:** `cache.Client` and `cache.CacheClientBound` (`pkg/cache/cache.go`, `pkg/cache/cacheClientBound.go`) each decompose into four capability interfaces — `IntentReader`, `IntentWriter`, `RunningStore`, `InstanceLifecycle` (named consistently across both, with `CacheClientBound`'s versions using its own bound-call method names) — recomposed via embedding back into the exported `Client`/`CacheClientBound` names, so every existing caller that depends on the full set keeps compiling unchanged. This is a pure interface restructuring: `LocalCache` and `ConfigServerCache` still implement the identical flat method set as before, so neither concrete type needs any code change to keep satisfying `Client`. Regenerate `mocks/mockcacheclient/*` (via the existing `mockgen -source=...` command) so mock types now exist per capability interface as well as for the composed `Client`/`CacheClientBound`.

**Blocked by:** None — can start immediately.

**Status:** done

- [x] `pkg/cache/cache.go` defines `IntentReader`, `IntentWriter`, `RunningStore`, `InstanceLifecycle`; `Client` embeds all four and is otherwise unchanged in method surface.
- [x] `pkg/cache/cacheClientBound.go` defines the bound-call equivalents of the same four capabilities; `CacheClientBound` embeds all four and is otherwise unchanged in method surface.
- [x] `CacheClientBoundImpl` requires no changes — it already implements every method needed.
- [x] Compile-time assertions confirm `*LocalCache` satisfies `Client` (and, by extension, all four capability interfaces) with zero code changes to `pkg/cache/local.go`.
- [x] `mocks/mockcacheclient/*` regenerated against the new interface shapes (mechanical; run the documented `mockgen -source=...` command from each file's header comment).
- [x] `go build ./...`, `go vet ./...`, and `go test ./...` all pass with zero behavior change.

## Comments

- Landed via TDD (compile-time `var _ Interface = (*Type)(nil)` assertions as the red→green seam, since this is a pure interface restructuring with no runtime behavior). `pkg/cache/cache.go` gained `IntentReader`/`IntentWriter`/`RunningStore`/`InstanceLifecycle`; `pkg/cache/cacheClientBound.go` gained the bound-call equivalents named `BoundIntentReader`/`BoundIntentWriter`/`BoundRunningStore`/`BoundInstanceLifecycle` (distinct names required since both live in package `cache`). New `pkg/cache/cache_test.go` holds the compile-time assertions. Mocks regenerated in `mocks/mockcacheclient/{client.go,clientbound.go}`. Zero code changes to `local.go`/`configserver.go`; full `go build`/`go vet`/`go test ./...` pass.
