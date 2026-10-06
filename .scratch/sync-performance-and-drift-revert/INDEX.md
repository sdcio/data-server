# Sync performance + precise Drift revert

**Parent spec:** [11-sync-performance-and-drift-revert-spec.md](../open/11-sync-performance-and-drift-revert-spec.md)  
**Delivery:** one small PR per ticket, each with before/after benchmark numbers. Based on `main`; datastore-level tickets (04–06) rebase over PR [#501](https://github.com/sdcio/data-server/pull/501) when it merges.

## How to use this index

Say: **"implement the next item from `.scratch/sync-performance-and-drift-revert/INDEX.md`"**.

The agent should:

1. Read this file and pick the first ticket in **Frontier** (lowest number) unless you name one.
2. Read that ticket file under `issues/` and the parent spec.
3. Put the work on a branch named for **this** ticket: `sync-perf/<NN>-<slug>` (NN is the ticket number). Base that branch on the **Blocked by** parent’s branch (the parent in the table). If **Blocked by** is `—`, base it on `main`. If there are several blockers, base it on a branch that already contains all of them; if none exists, stop and ask. This step is done when `git branch --show-current` contains this ticket’s number and every **Blocked by** commit is an ancestor (`git merge-base --is-ancestor <blocker> HEAD`).
4. Implement it on that branch, meeting every acceptance criterion (including benchmark before/after numbers).
5. Update this index: set the ticket **Status** to `done` in the table, check **Done**, remove it from **Frontier**, and move any newly unblocked tickets from **Backlog** to **Frontier**. Set the ticket file's **Status** to `done` too.

## Frontier (ready to start)

- [06 — Changed-path Drift comparison](issues/06-changed-path-drift-comparison.md)
- [07 — Characterise delete-path owner-variant behaviour](issues/07-characterise-delete-path-owner-variant.md)
- [09 — Snapshot-slice walkers](issues/09-snapshot-slice-walkers.md)
- [10 — Decide on gNMI direct apply / tree-to-tree import](issues/10-decide-gnmi-direct-apply.md)

## Backlog (blocked)

| # | Blocked by |
|---|------------|
| 08 | 01, 07 |

## All tickets

| # | Title | Blocked by | Status | Done |
|---|-------|------------|--------|------|
| 01 | [Benchmark suite and lock-hold metric](issues/01-benchmark-suite-and-lock-hold-metric.md) | — | done | [x] |
| 02 | [Direct child lookups, empty-children fast path, `-race` in CI](issues/02-direct-child-lookups-and-race-ci.md) | 01 | done | [x] |
| 03 | [Touched-entry scoping of remove-deleted and reset-flags](issues/03-touched-entry-cleanup-scoping.md) | 01 | do-not-ship | [x] |
| 04 | [Coarse Drift revert gate + Outstanding drift revert marker](issues/04-coarse-drift-revert-gate.md) | 02 | done | [x] |
| 05 | [Drift revert failure visibility + copy out of lock](issues/05-drift-revert-failure-visibility-and-copy-out-of-lock.md) | 04 | done | [x] |
| 06 | [Changed-path Drift comparison](issues/06-changed-path-drift-comparison.md) | 05 | ready-for-agent | [ ] |
| 07 | [Characterise delete-path owner-variant behaviour](issues/07-characterise-delete-path-owner-variant.md) | — | ready-for-agent | [ ] |
| 08 | [Lazy delete-path coverage](issues/08-lazy-delete-path-coverage.md) | 01, 07 | ready-for-agent | [ ] |
| 09 | [Snapshot-slice walkers](issues/09-snapshot-slice-walkers.md) | 02 | ready-for-agent | [ ] |
| 10 | [Decide on gNMI direct apply / tree-to-tree import](issues/10-decide-gnmi-direct-apply.md) | 02 | ready-for-agent | [ ] |

### Status values

- `ready-for-agent` — not started (blockers may still be open; see **Blocked by**)
- `in-progress` — someone is working on it
- `done` — acceptance criteria met
- `do-not-ship` — tried; closed without merging (see the ticket)

A ticket is startable only when every ticket in its **Blocked by** is `done` or `do-not-ship`.
