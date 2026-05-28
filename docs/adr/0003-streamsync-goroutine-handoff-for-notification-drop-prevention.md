# ADR 0003: StreamSync goroutine handoff to prevent post-SyncResponse notification drops

## Status

Accepted — 2026-05-28

## Context

`StreamSync.buildTreeSyncWithDatastore` ran a single `for/select` loop that both drained the incoming notification channel (`updChan`) **and** called `syncToRunning` inline. `syncToRunning` calls `ApplyToRunning` → `performRevert`, which loads all non-running intents, validates the merged tree, and pushes revert deltas to the device. This can take several seconds.

While `syncToRunning` held the goroutine, no notifications could be consumed from `updChan`. Pool workers delivering notifications blocked until `notificationSendTimeout` (5 s) expired, at which point the update was silently and permanently dropped. Any notification dropped during this window left the running view of the device permanently incomplete until the datastore was recreated.

A concurrent race (problem B) compounded this: the `SyncResponse` signal was sent directly to the consumer goroutine while in-flight pool workers were still writing their notifications to `updChan`. Some post-`SyncResponse` onChange notifications arrived while the goroutine was blocked in `syncToRunning`, causing the drops.

A real CI failure confirmed the impact: the mandatory `password` leaf for the `admin` user was permanently absent from the running tree, causing every subsequent `TransactionSet` validation to fail for 34+ minutes.

## Decision

**Primary fix (Problem A):** apply a goroutine handoff in `buildTreeSyncWithDatastore`.

On each `syncResponse` or ticker commit event, the current `syncTree` is snapshotted as `treeToCommit`, `syncTree` is immediately reset to a fresh empty tree (from `NewEmptyTree`), and `syncToRunning` is called in a background goroutine. The main loop never blocks on a commit — it keeps draining `updChan` at all times.

Key implementation details:
- An `atomic.Bool syncRunning` tracks whether a commit goroutine is in flight. The ticker case skips if `syncRunning` is true (the running goroutine will commit the accumulated state). The `syncResponse` case always spawns since the initial sync commit must not be skipped.
- Both cases set `syncRunning.Store(true)` before spawning; the goroutine calls `defer syncRunning.Store(false)`.
- If `NewEmptyTree` fails, `buildTreeSyncWithDatastore` logs and returns rather than continuing with a shared tree that is also in use by the spawned goroutine.
- The local `syncTreeMutex` previously passed to `syncToRunning` is removed: with the handoff, each goroutine owns its own snapshot so there is no shared mutable local tree. Concurrent calls to `ApplyToRunning` are serialized by `d.syncTreeMutex` inside the datastore, which was always the authoritative guard.
- The `syncToRunning` signature is simplified: it no longer returns a `*tree.RootEntry` (the fresh tree is created in the main loop before spawning) and no longer accepts a `*sync.Mutex` parameter.

**Problem B (post-SyncResponse notification lag):** with Problem A fixed, notifications that race past the `SyncResponse` signal land in the fresh `syncTree` and are committed by the next 5-second ticker. This is a bounded, self-healing lag (≤ 5 s) rather than permanent loss. A full per-window drain (Option 4 in the issue) was considered but deferred: it adds pool lifecycle complexity and the bounded lag is acceptable given that `ApplyToRunning` is idempotent and the ticker commits continuously.

## Alternatives considered

- **Increase `updChan` buffer** — raises the threshold at which drops occur, does not eliminate the root cause.
- **Remove drop timeout** — pushes the problem upstream; a stuck `performRevert` would freeze the whole stream pipeline instead of logging a drop.
- **Per-window drain (`CloseAndWait`)** — eliminates the 5-second Problem B lag but must be combined with the goroutine handoff to be safe; deferred as a follow-up.
- **Decouple `performRevert` from `ApplyToRunning`** — architecturally cleanest long-term direction (the `// TODO` comment in `sync.go` already flags this); larger refactor, tracked separately.

## Consequences

- **Positive:** Post-`SyncResponse` notifications are never permanently dropped regardless of how long `performRevert` takes.
- **Positive:** The `syncToRunning` signature is simpler (no mutex parameter, no tree return value).
- **Negative:** A bounded lag (≤ 5 s) remains for the narrow race where post-`SyncResponse` onChange notifications arrive before the ticker fires; acceptable given idempotent apply and continuous ticker commits.
- **Observability:** Any `NewEmptyTree` failure now causes `buildTreeSyncWithDatastore` to exit and log, surfacing a previously silent degraded state.

## Related

- [GitHub issue #440](https://github.com/sdcio/data-server/issues/440)
