# 03 — Replace `noopIntentWriter` with a real `IntentWriter` for `Cache.Type: config-server`

**Spec:** `config-server` repo, `.scratch/last-applied-snapshot-write-at-apply/spec.md`; see also `pkg/cache/docs/adr/0003-config-server-write-path-real-last-applied-writes.md` (already drafted, uncommitted in this worktree)

**What to build:** `IntentModify`/`IntentDelete`, called from `lowlevelTransactionSet`'s apply loop (the same call sites `Cache.Type: local` already uses), now perform real writes against `ConfigSnapshotService.Modify`/`Delete` — synchronously, before the transaction response returns. A failed `Delete` RPC hard-fails the transaction, matching `Modify`'s existing behavior (this is a deliberate asymmetry fix vs. the pre-fix log-only delete failure).

**Blocked by:** 02 (config-server write handlers must exist to call)

- [x] `ConfigServerCache`'s `Client` composition (`pkg/cache/configserver.go` / `NewConfigServerClient`) drops `noopIntentWriter` and composes a real writer calling `Modify`/`Delete`
- [x] `noop_intent_writer.go` and `noop_intent_writer_test.go` themselves are **left in place** (per ADR 0003 — still valid for a future genuinely read-only backend); only the config-server composition point changes
- [x] Unit test: delete-apply removes the last-applied entry immediately; the next `LoadAllButRunningIntents` does not rehydrate it (the ghost-intent regression signature) — `TestConfigServerBackend_DeleteApply_NoRehydration` (`pkg/datastore/transaction_rpc_test.go`, wired against a real `*cache.ConfigServerCache` over `configserver.FakeLocalConfigClient`, not a generic mock) drives `LoadAllButRunningIntents` directly; `TestConfigServerCache_InstanceIntentDelete_RemovesFromSeam` (`pkg/cache/configserver_test.go`) covers the same removal at the narrower seam level
- [x] Unit test: modify-apply is visible on next read — `TestConfigServerCache_InstanceIntentModify_CreatesAndIsReadableBack` (`pkg/cache/configserver_test.go`)
- [x] Unit test: rollback (`TransactionSet` re-run on old intents) restores a deleted entry — `TestTransactionRollback_RestoresDeletedIntent` (`pkg/datastore/transaction_rpc_test.go`); asserts both the intent name and the restored leaf content, not just that some `IntentModify` call happened
- [x] Unit test: a failed `Delete` RPC hard-fails the transaction (regression test for the asymmetry fix) — `TestTransactionSet_IntentDeleteFailureHardFailsTransaction` (`pkg/datastore/transaction_rpc_test.go`); fix itself is in `pkg/datastore/transaction_rpc.go`'s `IntentDelete` error branch (was log-only, now returns the wrapped error)
- [x] Now-wrong no-op assertions in `pkg/cache/configserver_test.go` and `pkg/server/cache_test.go` updated to assert real writes instead
- [x] Regenerate `mocks/mockconfigread` against the renamed `ConfigSnapshotServiceClient` (rename dir/mock accordingly) and update the `mockgen` line in `Makefile`; tests above consume the regenerated mock
- [x] Commit the already-drafted `pkg/cache/docs/adr/0003-...md` and the `pkg/cache/CONTEXT.md` "Last-applied" update sitting uncommitted in this worktree, as part of this change
