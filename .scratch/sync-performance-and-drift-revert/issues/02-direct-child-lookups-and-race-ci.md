# 02 — Direct child lookups, empty-children fast path, `-race` in CI

**What to build:** Looking up a child by name no longer copies the sibling map, so inserting into a wide list costs time independent of list width. Lookups over active children still honour the choice skip list with identical results. Getting the children of an entry with no children returns without allocating (the ordering helper tolerates an empty value). CI runs the race detector on the tree and datastore packages. Also fix the key-listing re-entrant read lock deadlock opportunistically.

**Blocked by:** 01 — Benchmark suite and lock-hold metric

**Status:** done

- [x] Insert-scaling benchmark shows roughly flat per-insert cost from 100 to 10,000 interfaces
- [x] Existing tree, ops, processors and validation tests pass unchanged
- [x] Choice/case active-children lookups return the same results as before
- [x] Leaf entries return children without allocating
- [x] `go test -race` on tree and datastore packages runs in CI and is clean
- [x] Key listing no longer takes the read lock twice
- [x] Before/after benchmark numbers stated in the PR

**Benchmark (`BenchmarkAddUpdatesRecursive`, `-benchmem -benchtime=3x`, add one interface into a tree of N):**

| N | Before (ticket 01 / BASELINE) | After |
|---|---|---|
| 100 | 0.11 ms | 0.146 ms (696 allocs, 86 KB) |
| 1,000 | 1.1 ms | 0.132 ms (695 allocs, 74 KB) |
| 10,000 | 10 ms (~7.2 MB) | 0.242 ms (695 allocs, 74 KB) |

Per-insert cost is now roughly flat with list width (n=10,000 is ~1.7× n=100, not 100×). The n=100 before/after times are in the same noise band; the win is at width.
