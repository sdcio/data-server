# Under `Cache.Type: config-server`, `IntentModify`/`IntentDelete` become real writes at apply time — superseding ADR 0001's no-op clause

**Supersedes:** [ADR 0001](0001-config-server-backed-cache-client.md)'s decision that `InstanceIntentModify`/`InstanceIntentDelete` are "unconditional silent no-ops" under `Cache.Type: config-server`. Every other part of ADR 0001 (the read seam, `"running"` split, field mapping) stands unmodified.

**Pairs with:** config-server `.scratch/last-applied-snapshot-write-at-apply/spec.md` (execution plan / implementation decisions for the config-server side of this same fix).

## Why ADR 0001's no-op clause was wrong

ADR 0001 reasoned that "config-server/kube-api is the sole writer" for real Intents, so a call to `IntentModify`/`IntentDelete` under this backend had "nothing actionable to do." That's true for *authorship* (who decides an Intent's desired content) but conflates it with *last-applied tracking* (what this datastore actually pushed south) — a second, distinct piece of state that `Cache.Type: local` has always updated at the exact moment of southbound apply, inside `lowlevelTransactionSet`'s apply loop, never gated on anything downstream.

Under `Cache.Type: config-server`, that second write was dropped on the floor. Config-server's own last-applied record (`TargetSnapshot.Spec.Configs`) was written only after `TransactionConfirm`, via `saveSnapshot` — a best-effort, non-fatal, unrelated-process write that could lag apply by an unbounded amount (a full reconcile round-trip, or longer if `saveSnapshot` failed and only retried on the next unrelated reconcile). CI caught the resulting race concretely: a deleted `intent1` remained "last-applied" in `TargetSnapshot` past its actual southbound deletion, got rehydrated by the next `LoadAllButRunningIntents`, and failed leafref validation during a *different*, unrelated `customer` ConfigSet delete — hanging that teardown for the CI timeout. The same lag independently produces false deviations and incorrect apply-vs-confirm-window reads for `BlameConfig`/`GetIntent`/revert, and risks replaying already-deleted config after a crash-restart (recovery replays only intents still present in the last-applied record).

An earlier, separate design pass (`target-snapshot-backed-config-read` spec, which repointed `ConfigReadService` reads to `TargetSnapshot`) had concluded a stale snapshot was harmless — "worst case a redundant re-push." **That conclusion is retracted.** It was reached before this bug's evidence existed; a stale-behind last-applied record can rehydrate genuinely deleted config into the validation tree, which is a correctness failure, not a harmless no-op.

## The fix

`Cache.Type: config-server`'s `IntentWriter` stops being `noopIntentWriter`. `IntentModify`/`IntentDelete`, called from the same apply-loop call sites `Cache.Type: local` already uses, now perform a real write — synchronously, inside `TransactionSet`, before the response reaches the caller:

- **`IntentModify`** upserts the applied, encrypted payload for that Intent name into config-server's last-applied record.
- **`IntentDelete`** removes that Intent's entry from the same record.
- **Membership rule:** an Intent is last-applied **iff its entry is present**. Delete removes the entry outright — no tombstone, deleted-annotation, or deleted-label scheme. Simpler mental model, no second "is this really gone" check for every reader (`ConfigReadService`, `LoadAllButRunningIntents`, recovery) to carry, and it matches `Cache.Type: local`'s existing all-or-nothing disk-key semantics exactly.
- **Rollback restores it.** Timeout/cancel rollback re-runs `TransactionSet` on the old intents; those `IntentModify` calls restore any entry a rollback needs to bring back. Last-applied tracking is symmetric with device-state rollback — it was never optional just because the record now lives in config-server instead of on local disk.
- **A failed delete-write must hard-fail the transaction**, matching `IntentModify`'s existing behavior. The pre-fix code left `IntentDelete` failures log-only; keeping that asymmetry here would mean a failed write leaves a silent ghost entry — the exact bug this ADR fixes, just relocated one layer down into "the write path's own failure mode." If the write can silently fail, "last-applied only changes at apply time" isn't actually true.

Config-server's post-`TransactionConfirm` `saveSnapshot` is **demoted to a backstop**: it may prune entries whose upstream config no longer exists, and refresh incidental metadata (schema hashes), but it is no longer the authority for membership, must never run on a failed/rolled-back transact, and must never full-replace the record in a way that clobbers what the apply-time write (or a rollback) just wrote. See the config-server-side spec for the mechanics (a targeted per-key patch, not get-then-replace, so the apply-time write and the backstop prune can't stomp on each other).

## Wire contract: one merged service, not read/write split

The original local RPC surface (`ConfigReadService`, `sdc-protos/config_read.proto`) was deliberately read-only, because ADR 0001 assumed writes would always be no-ops. That assumption no longer holds, and two options were considered for exposing the new write capability:

1. **A new sibling service** (e.g. a `ConfigSnapshotWriteService` next to the existing `ConfigReadService`), keeping "read" and "write" as separately named, separately registered gRPC services.
2. **One merged service** covering `Get`/`List`/`Modify`/`Delete`, replacing `ConfigReadService` entirely (renamed, since it's no longer read-only).

**Decision: merge (option 2).** This is a single colocated, single-consumer, localhost-bound sidecar API — one pod, one trust boundary, one backing resource (`TargetSnapshot`), one client (`ConfigServerCache`). There's no multi-tenancy, independent versioning, or security-boundary reason for two services here; two services would just mean two proto files, two server registrations, and two client dials for what is fundamentally one capability surface over one resource. Splitting by capability is still valuable — it's just enforced one layer up, in data-server's Go `cache.Client` interface (`IntentReader`/`IntentWriter`, already segregated per the capability-split refactor this backend's `Client` composition depends on), not in the wire proto. A single generated gRPC client structurally satisfies both narrower Go interfaces regardless of how many RPCs live on one proto service, so merging the wire surface costs nothing at the Go layer that actually needs the segregation for testing/mocking.

Keeping the old name while adding `Modify`/`Delete` to it was explicitly rejected: `ConfigReadService`'s own doc comment says "a unary, localhost-bound **read** API" — leaving that name on a service that now writes would be the same stale-name-vs-behavior mismatch this ADR spends most of its text fixing for "confirmed" vs "last-applied." The renamed, merged service is `ConfigSnapshotService` (`Get`/`List`/`Modify`/`Delete`), matching the `TargetSnapshot` resource it fronts and the `last-applied` vocabulary this ADR establishes.

### RPC shape

- **Per-intent, not batched.** One `Modify`/`Delete` call per intent, matching today's apply-loop call shape exactly (one call site, no new batching/aggregation logic to design or test). Batching per-`TransactionSet` is a legitimate future optimization if per-intent round-trip latency ever matters, not a blocker here.
- **Payload crosses the wire, not a re-read signal.** `IntentModify`'s RPC carries the actual applied payload (the same `protoIntent` `ops.TreeExport` already built for this Intent) — config-server stores that content directly, filling incidental fields (`Revertive`/`Lifecycle`) from its own current `SensitiveConfig`. The alternative (data-server sends only "Intent X applied," config-server re-reads its current desired `SensitiveConfig` to build the record) was rejected: it reintroduces the exact live-vs-last-applied conflation this whole fix exists to close, since config-server's "current" desired value can already differ from what was actually pushed by the time the RPC arrives.

## Consequences

- `noopIntentWriter` (`pkg/cache/noop_intent_writer.go`) is no longer composed for `Cache.Type: config-server`. It remains available in `pkg/cache` for a genuinely read-only future backend to compose — the type itself isn't wrong, only its use here was.
- Every consumer of last-applied state (`LoadAllButRunningIntents`, `calculateDeviations`, `BlameConfig`, `GetIntent`, `performRevert`, crash recovery) gets correct apply-time semantics with no call-site changes — they already read through `Client.InstanceIntent*`/`ConfigReadService`; only what's behind that read changed.
- `pkg/cache/CONTEXT.md` gains the **Last-applied** term, distinct from **Intent** (desired/authored). See that file for the full definition.
- This does not reopen ADR 0001's read-path decisions (the `ImportConfigAdapter` field mapping, the `"running"` split, `ExplicitDeletes`'s documented always-empty limitation) — those stand.
