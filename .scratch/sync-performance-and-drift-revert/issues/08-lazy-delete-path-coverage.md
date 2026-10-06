# 08 — Lazy delete-path coverage

**What to build:** A delete-path intent is held as coverage (owner, priority, path) in the tree context instead of one synthetic explicit-delete entry per covered leaf. Precedence treats the covering delete as a virtual variant with identical semantics, and every place that asks whether the highest variant is an explicit delete sees it. Tree debug output prints one marker on the covered branch (owner, priority, "explicit delete, covers subtree") with an optional expanded per-leaf mode. A delete-path pointing at a path not in the tree is skipped with a warning. The list of created explicit-delete entries is replaced by a count.

**Blocked by:** 07 — Characterise delete-path owner-variant behaviour

**Status:** done

- [x] Characterisation tests from 07 pass unchanged
- [x] Tree string output is unchanged for trees without delete-path intents (existing comparison tests untouched)
- [x] Covered branch prints once; expanded mode shows the effective per-leaf view
- [x] Delete-path with a missing path is skipped with a warning, no crash
- [x] Deviation and transaction behaviour unchanged
- [x] Finish-insertion with delete-path on `/` no longer allocates per covered leaf (benchmark)
- [x] Before/after benchmark numbers stated in the PR

## Benchmark

Command: `go test ./pkg/datastore -run '^$' -bench 'BenchmarkSyncStages/finish-insertion/n=10000' -benchmem -benchtime=3x -count=1`

- Before (ticket-01 baseline): no delete path `106 ms`, `0.28M allocs/op`; delete path on `/` `292 ms`, `2.25M allocs/op` (`+186 ms`, `+1.97M allocs/op`).
- After (this branch): no delete path `287.9 ms`, `3,270,143 allocs/op`; delete path on `/` `255.2 ms`, `3,270,150 allocs/op` (`-32.7 ms`, `+7 allocs/op`).

The absolute after numbers include the current branch's per-iteration benchmark setup; the paired result shows that delete-path coverage no longer adds allocations proportional to the covered leaves.
