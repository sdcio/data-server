# Bug: Sync Revert Races with In-Flight Transaction, Re-applies Deleted Config

## Status

`done`

## Problem Statement

The periodic **Sync** loop and a **Transaction** can run concurrently for the same **Datastore**. When a **Transaction** deletes an intent's config from a **Target**, there is a short window between the **Target** applying the change internally and the **Transaction** completing its cache update. If the sync loop fires in that window, reads the device (sees the config already gone), and then compares against the intent cache (still has the intent as active), it concludes the config is "missing" and pushes it back to the device via `performRevert`. The **Transaction** then finishes and removes the intent from the cache — but the device now has the config back, and no further sync cycle will remove it (it is now **Unhandled** / unmanaged, so the system correctly refuses to auto-delete it). The net result is a permanent configuration leak on the device until the next intentful operation touches that path.

This bug was found by analysing a failing CI integration test (`Delete And Verify intent4` in `tests/02-crud/11-sros-create-delete.robot`, GitHub Actions run `27938669302`). It recurred as `Delete And Verify intent2` (`vprn234` on `sr2`) in run `35592679198` under the config-server cache backend job. Evidence is fully in the CI artifact logs.

## Root Cause: Detailed Timeline

**Actors:**

- `TransactionSet/transaction` (`f1f15585-f004-42f2-a989-802db9b3d057`) — deleting `intent4-sros` (`vprn987` on `sr2`)
- `datastore/sync` — periodic 5 s sync loop for `default.sr2`
- `datastore/DeviationManager` — companion component (NOT the bug, see below)

**Event sequence (all within the same log-second `08:57:08Z`, ordered by log line number):**

| Line | Component | Event |
|------|-----------|-------|
| 717 | TransactionSet | Sends gNMI `delete /configure/service/vprn[service-name=vprn987]` to sr2 |
| 718 | DeviationManager | `deviation calc run - start` |
| 719 | sync | `syncing` — sends gNMI GET to sr2 |
| 722–731 | DeviationManager | Loads intents (incl. `intent4-sros`), compares against **synctree** → finds `intent4-sros: {IntentExists: 1}`, no deviation; finishes |
| 732 | sync | Receives GET response — **device already shows no `vprn987`** |
| 733 | sync | Applies device state to synctree |
| 734–737 | sync | Adds `customer-sr2` and **`intent4-sros`** to comparison tree — **intent4-sros still in cache** |
| 738 | sync | **`reverting after sync`** — device missing `vprn987` but desired |
| 739 | sync | Sends gNMI SET update pushing `vprn987` back to device |
| 751 | TransactionSet | Receives DELETE SetResponse (too late — revert already sent) |
| 759 | TransactionSet | Deletes `intent4-sros` from intent cache |
| 760 | TransactionSet | Writes back synctree (no `vprn987`) |
| 761–767 | TransactionSet | Transaction confirmed and cleaned up |
| 772 | sync | Receives revert SetResponse — **`vprn987` is now back on device** |

### Why the DeviationManager is NOT involved

`datastore/DeviationManager` (lines 718–731) runs against the in-memory **synctree**, which still had `vprn987` (the transaction had not yet written back). It found no deviation and finished cleanly before the sync received its GET response. The revert was triggered exclusively by the sync loop's `performRevert` path in `pkg/datastore/sync.go`.

### Why only this one test instance failed

The sync fires every 5 seconds: `08:56:48`, `08:56:53`, `08:56:58`, `08:57:03`, **`08:57:08`**, `08:57:13` ...

- `intent1` delete (`vprn123`) completed at `08:57:05Z` — **3 seconds before** the sync; cache was already updated when the sync ran, no false deviation.
- `intent4` delete (`vprn987`) was in flight at `08:57:07–08Z` — landed exactly on the sync boundary. Pure timing.

### Post-race state

From `08:57:18Z` onward, deviation stats show `running: {Unhandled: 225}` (up from 221 before). The 4 extra entries are `vprn987`'s leaf paths — on the device but owned by no intent. The system correctly does not auto-delete unmanaged config, so `vprn987` persists for the full 2-minute `Wait Until Keyword Succeeds` window, causing the test assertion `Should Be Empty` to fail.

## Affected Code

### `pkg/datastore/sync.go` — `performRevert`

`performRevert` is called from `ApplyToRunning` (the sync completion path). It loads the current intent cache, computes deltas vs. the device state it just read, and may push corrections to the device via `applyIntent`.

**Locking gap (pre-fix):** `TransactionSet` / `Confirm` / `Cancel` take `Datastore.dmutex` (fail-fast `TryLock`). Sync Revert did **not**. "One transaction at a time" meant one northbound Transaction RPC, not one southbound device Set. `IntentDelete` runs only after the delete Set returns, so a concurrent sync still saw the intent as desired.

### `pkg/datastore/types/transaction_manager.go` — `TransactionManager`

Tracks the registered Northbound Transaction for Confirm/Cancel/rollback identity. It is **not** the southbound mutual-exclusion lock; that role belongs to `dmutex`.

## Implemented Fix

Guard Sync Revert's southbound apply with the same `dmutex` Northbound Transactions already use. Prefer `TryLock` + skip over blocking `Lock()`: this cycle's device snapshot may already be mid-flight junk, so waiting then applying that copy is wrong — abandon the cycle and reevaluate on the next sync GET.

In `pkg/datastore/sync.go`, `performRevert`:

```go
if performApply {
    if !d.dmutex.TryLock() {
        log.V(logger.VDebug).Info("skipping revert after sync: datastore locked")
        return nil
    }
    defer d.dmutex.Unlock()

    log.Info("reverting after sync")
    resp, err := d.applyIntent(ctx, adapter.NewEntryOutputAdapter(t.Entry))
    ...
}
```

This is safe because:
- When a Transaction holds `dmutex`, Sync Revert skips immediately; the next sync cycle (after IntentDelete) sees consistent desired state.
- When Sync Revert holds `dmutex`, a concurrent `TransactionSet` gets `ErrDatastoreLocked` (same fail-fast model as two Transactions colliding).
- Blocking `Lock()` was rejected: the GET already happened without exclusivity; blocking then applying the stale copy can still mis-revert.

### Why not `IsTransactionOngoing` alone

An earlier draft proposed skipping when `TransactionManager` reports an ongoing transaction. That covers most of the TransactionSet window but has a TOCTOU gap (check false → both Sync and Transaction start apply) and does not serialize Sync Revert against other `dmutex` holders. `dmutex.TryLock` is the universal southbound set-path lock.

### What the fix does NOT change

- The `DeviationManager` logic — it is correct.
- Transaction processing, validation, or rollback paths.
- The sync interval or the sync's GET/import path; only the revert apply step is guarded.
- End-to-end exclusive GET+revert (would need a larger redesign); this fix only prevents concurrent southbound Sets from a tainted revert cycle.

## Testing

1. **Unit test** `TestPerformRevert_SkipsSouthboundApplyWhenDatastoreLocked` — hold `dmutex`, call `performRevert` with desired ≠ running, assert `Target.Set` is never called.
2. **Unit test** `TestPerformRevert_AppliesWhenDatastoreUnlocked` — free `dmutex`, same setup, assert `Target.Set` is called once.
3. **Integration soak**: existing `11-sros-create-delete.robot` still exercises the path probabilistically; a dedicated forced-race IT was deferred (no pause/hold seam today).

## Files changed

| File | Change |
|------|--------|
| `pkg/datastore/sync.go` | `dmutex.TryLock` around Sync Revert's `applyIntent`; skip when locked |
| `pkg/datastore/sync_test.go` | Locked-skip + unlocked-apply regression tests |

## Evidence

All log lines referenced above are from the CI artifact for run `27938669302`:

- `02-crud-logs/data-server.log` lines 705–773
- `02-crud-out.xml` test `id="s1-s1-t8"` (`Delete And Verify intent4`)

Recurrence: run `35592679198` / `Delete And Verify intent2` / `vprn234` on `sr2`.

The artifacts were downloaded to `/tmp/ci-artifact/` on the investigation machine during the analysis session documented in transcript [`sync-revert race investigation`](8da4f4d6-f3d5-49ab-a617-f434c1639d1e).
