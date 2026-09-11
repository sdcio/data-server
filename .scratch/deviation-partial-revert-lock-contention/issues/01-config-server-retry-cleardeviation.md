# 01 — config-server: retry `executeClearDeviationTx` on recoverable gRPC errors

**Status:** done

**What was built:** `apis/config/target_helpers.go`'s `executeClearDeviationTx`
(reached by `kubectl sdc deviation --revert`, including `--filter-path` partial
reverts, via the `cleardeviation` k8s subresource) now retries its
`TransactionSet`/`TransactionConfirm` calls up to 5 times with a fixed 500ms
backoff when the error is `dsclient.IsRecoverableError` (`codes.Aborted` /
`codes.ResourceExhausted`) — the same classification and backoff
`TargetConfigController`'s reconcile loop already uses for its own
`TransactionSet` calls (`pkg/reconcilers/targetconfig/reconciler.go`).

`isRecoverableGRPCError` (`pkg/sdc/target/manager/transactor.go`) was moved to
`pkg/sdc/dataserver/client.IsRecoverableError` — the lowest-layer package both
`apis/config` and `pkg/sdc/target/manager` already import — to share the
classification without an import cycle (`apis/config` cannot import
`pkg/sdc/target/manager`, which itself imports `apis/config`).

**Blocked by:** none.

**Verification:** `go build ./...`, `go vet ./apis/config/... ./pkg/sdc/...`,
and `go test ./apis/config/... ./pkg/sdc/target/manager/...` all pass on
config-server (branch `config-server-cache-backend`).

## Comments

Repo: `config-server`. Files touched:
- `pkg/sdc/dataserver/client/client.go` (+`IsRecoverableError`)
- `pkg/sdc/target/manager/transactor.go` (delegate to the shared helper)
- `apis/config/target_helpers.go` (+`retryOnRecoverable`, wraps both RPC calls
  in `executeClearDeviationTx`)
