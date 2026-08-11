# 03 — Delete `CacheConfig.Namespace` config field and its validation

**What to build:** `Cache.Type: config-server` requires no `namespace` setting in the deployment config at all, so an operator can't misconfigure a value that was never meaningful once namespace became per-datastore (derived from the datastore name itself, per ticket 02). `CacheConfig.Namespace` is deleted entirely, along with its yaml/json tags and the `c.Namespace == ""` requirement in `CacheConfig.validateSetDefaults()`'s `config-server` case. `config-server` type validation still requires `Address`.

**Blocked by:** 02

**Status:** done

- [x] `CacheConfig.Namespace` field (and its yaml/json tags) removed from `pkg/config/datastore.go`
- [x] `CacheConfig.validateSetDefaults()`'s `config-server` case no longer checks/requires a namespace, still requires `Address`
- [x] `pkg/server/cache.go`'s config-server call site (and its log line) no longer references `s.config.Cache.Namespace`
- [x] `pkg/config/datastore_test.go`: the three `Namespace: "sdcio"`-bearing cases updated — remove the now-nonexistent "config-server type requires a namespace" case, and drop `Namespace: "sdcio"` from the other two literals; confirm `config-server` type validation still requires `Address` but no longer mentions `Namespace` at all
- [x] `pkg/server/cache_test.go`: `TestCreateConfigServerCacheClient`/`TestCreateConfigServerCacheClient_WritesAreNoOps` updated to drop `Namespace: "sdcio"` from the `CacheConfig` literal (construction succeeds without it)
- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass

## Comments

- Landed in `pkg/config/datastore.go` (+ `pkg/config/datastore_test.go`, `pkg/server/cache.go`, `pkg/server/cache_test.go`): `CacheConfig.Namespace` and its validation deleted; `config-server` type now only requires `Address`.
