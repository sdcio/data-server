# 03 — Narrow read-only consumers to `IntentReader`

**What to build:** `forEachIntent` (`pkg/datastore/transaction_rpc.go`) and `populateSensitivePathIndex` (`pkg/datastore/datastore_rpc.go`) narrow their parameter type from `cache.CacheClientBound` to `cache.IntentReader`, since both only ever read real Intents. `Datastore`'s own `cacheClient` field stays a single `cache.CacheClientBound` — no struct changes needed; Go's structural typing lets the narrower parameter types accept it as-is at every existing call site. Tests exercising these two functions (`pkg/datastore/transaction_rpc_test.go`, `sync_test.go`, `sensitive_path_union_test.go`, `datastore_test.go`) construct a narrower `IntentReader` mock instead of a full `CacheClientBound` mock wherever they only need to satisfy these two call paths — a shrinking mock-setup diff here is the concrete signal the split is paying off, not just a type-signature change.

**Blocked by:** 01 — needs the `IntentReader` interface and its regenerated mock type.

**Status:** done

- [x] `forEachIntent`'s `cc` parameter is typed `cache.BoundIntentReader` instead of `cache.CacheClientBound`. (Note: the ticket text said `cache.IntentReader`, but `forEachIntent` calls the bound-call form `cc.IntentGetAll(ctx, exclude, ...)` with no `cacheName` argument — that's `cache.BoundIntentReader`'s shape, not the unbound `cache.IntentReader`'s `InstanceIntentGetAll`. Used the interface that actually matches the call site.)
- [x] `populateSensitivePathIndex`'s `cc` parameter is typed `cache.BoundIntentReader` for the same reason.
- [x] No changes to `Datastore`'s `cacheClient` field type or any other caller of these two functions — existing call sites (`LoadAllButRunningIntents`, `datastore_rpc.go`'s caller) compile unchanged since `cache.CacheClientBound` structurally satisfies `cache.BoundIntentReader`.
- [x] `transaction_rpc_test.go` and new `datastore_rpc_test.go` gained direct unit tests of `forEachIntent`/`populateSensitivePathIndex` built against `mockcacheclient.MockBoundIntentReader` (not `MockCacheClientBound`) — these are the first tests to exercise the two functions directly rather than only through `Datastore`, and they're the concrete "shrinking mock-setup diff" the split enables. `sync_test.go` and `sensitive_path_union_test.go` were left unchanged: every mock in those files is stored into `Datastore.cacheClient` (typed `cache.CacheClientBound`), so a narrower mock can't be substituted there without changing that field's type, which is explicitly out of scope.
- [x] `go build ./...`, `go vet ./...`, and `go test ./...` all pass.

## Comments

- Landed via TDD: added `TestForEachIntent_NarrowIntentReader`, `TestForEachIntent_PropagatesStreamError` (`transaction_rpc_test.go`) and `TestPopulateSensitivePathIndex_NarrowIntentReader` (new `datastore_rpc_test.go`), all built against `mockcacheclient.MockBoundIntentReader`. Confirmed red (mock didn't satisfy `cache.CacheClientBound`), then narrowed `forEachIntent`/`populateSensitivePathIndex`'s `cc` parameter to `cache.BoundIntentReader` to go green. Full `go build`/`go vet`/`go test ./...` pass.
