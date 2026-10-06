# 04 — Coarse Drift revert gate with Outstanding drift revert marker

**What to build:** After a sync, the Drift revert is skipped when Running changed nothing (new, updated, removed leaves and emptied branches; leaf-lists compared order-sensitively) and no Drift revert is outstanding. A per-datastore, in-memory, concurrency-safe Outstanding drift revert marker is set when a revert is needed or fails, and cleared only after the revert to the target succeeded. A steady-state sync therefore skips the Drift revert entirely, while a failed revert is retried on the next sync.

**Blocked by:** 02 — Direct child lookups, empty-children fast path, `-race` in CI (03 closed do-not-ship; this ticket does not need Touched-entry clean-up)

**Status:** done

Behavioural tests at the `ApplyToRunning` seam (mock cache client, mock target):

- [x] Steady state, nothing changed, with and without intents: no target call
- [x] Unmanaged config added by hand: no target call
- [x] Unmanaged config added under a delete-path intent: target receives the delete
- [x] Value changed by hand contradicting a revertive intent: target receives the intended value
- [x] Failed Drift revert: next sync with no further change retries
- [x] Concurrent syncs finishing together: the marker is not lost
- [x] Empty snapshot and nil importer keep working; `MarkSynced` is never skipped or delayed by the gate
- [x] First sync after restart evaluates full state
- [x] Before/after benchmark numbers stated in the PR (steady-state sync roughly halves)

## Benchmark (n=10,000, 3×, this branch vs ticket 01 baseline)

| Scenario | Before | After |
|---|---|---|
| Steady `/`, no intents | 1.69 s / 10.6M allocs | **924 ms** / 6.52M allocs |
| Steady `/`, matching intent | 2.03 s / 12.7M allocs | **938 ms** / 6.52M allocs |
