# 05 — Drift revert failure visibility and sync-tree copy out of the lock

**What to build:** A failed Drift revert is reported to whoever triggered the sync instead of only being logged. The sync-tree copy needed for a revert is no longer taken under the sync tree lock for its whole duration; the lock is held only for what needs consistency. The lock-hold benchmark shows the reduction.

**Blocked by:** 04 — Coarse Drift revert gate with Outstanding drift revert marker

**Status:** done

- [x] Device unreachable / edit rejected surfaces as an error to the sync caller, and the marker stays set
- [x] The `ApplyToRunning` contract (nil importer, nil paths, `MarkSynced` after a successful apply) is unchanged
- [x] Lock-hold-time benchmark shows shorter sync tree lock hold
- [x] `-race` clean on tree and datastore packages
- [x] Before/after benchmark numbers stated in the PR

## Benchmark (n=10,000, 3×, this branch vs ticket 04 on the same machine)

Write-lock hold drops by about the deep-copy cost. Wall-clock time is unchanged because the copy still runs; it no longer holds the exclusive sync-tree lock.

| Scenario | Before (write-lock hold) | After (write-lock hold) |
|---|---|---|
| First sync, matching intent | 1.34 s | **875 ms** |
| Drift: one leaf changed | 1.07 s | **908 ms** |
